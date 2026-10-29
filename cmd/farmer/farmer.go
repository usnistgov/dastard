package main

// FARMER
//
// Fast Arrow Routing and Multichannel Event Reorganizer
//
// 2026. Joe Fowler, NIST Boulder Labs.

// Run as `farmer directory1 [dir2 ...]` to monitor `directory1` and optionally other
// directories. Will run in parallel, finishing as each directory is complete and fully
// reorganized from its initial time-major format into the single-channel arrow files.

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/apache/arrow-go/v18/arrow/memory"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/fsnotify/fsnotify"
)

func main() {
	// os.Args[0] is the name of the program, so we grab everything after it
	args := os.Args[1:]

	if len(args) == 0 {
		fmt.Println("Usage: farmer dir1 [dir2 ...]")
		os.Exit(1)
	}

	// 1. Initialize an in-memory DuckDB engine
	db, err := sql.Open("duckdb", "")
	if err != nil {
		log.Print("Could not open DuckDB: ", err)
		return
	}
	defer db.Close()

	// 2. Install and load the nanoarrow community extension
	// DuckDB will dynamically fetch this tiny library to enable IPC writing support.
	_, err = db.Exec(`
		INSTALL nanoarrow FROM community; 
		LOAD nanoarrow;
	`)
	if err != nil {
		log.Fatal("Failed to load the nanoarrow extension: ", err)
		return
	}

	// Use a WaitGroup to keep the main program alive while the goroutines do their work.
	var wg sync.WaitGroup
	log.Printf("Starting %d workers for the directories...\n", len(args))

	// 3. Loop through each CLI argument
	for _, arg := range args {
		// Increment the WaitGroup counter for each goroutine we spawn
		wg.Add(1)

		// 3. Spawn the goroutine, passing the argument and a POINTER to the WaitGroup
		go organizeDirectory(arg, &wg, db)
	}

	// 4. Block the main thread until the WaitGroup counter reaches exactly 0
	wg.Wait()

	log.Println("✅ All workers completed successfully.")
}

// organizeDirectory is the function that runs concurrently for each input directory, reorganizing the
// live data into a channel-major order, which is far more convenient for later analysis.
func organizeDirectory(directory string, wg *sync.WaitGroup, db *sql.DB) {

	defer wg.Done()
	log.Printf("[START] Processing: %s\n", directory)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Print(err)
		return
	}
	defer watcher.Close()
	watcher.Add(directory)

	// Before we start watching for files being touched, catch up with existing files
	for {
		pattern := filepath.Join(directory, "*.arrows_timeorder")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			log.Print(err)
			return
		}
		for _, path := range matches {
			sortIPCFile(path, db)
		}
		// Once there are no files to sort, check whether the run is complete and needs to be shuffled
		if len(matches) == 0 {
			complete := filepath.Join(directory, "COMPLETE")
			if _, err := os.Stat(complete); err == nil {
				// file COMPLETE exists and is accessible. Shuffle it, and we're done
				log.Println("Found a COMPLETE file. Shuffling the directory")
				shuffleDirectory(directory)
				log.Printf("✅ Finished organizing directory %s\n", directory)
				return
			}
			break
		}
		log.Printf("✅ Processed %d pre-existing *.arrows_timeorder files\n", len(matches))
	}

	// If we get here, the existing files are sorted, and COMPLETE doesn't exist, so there's one
	// last check: is this entire directory completely done already (no *.arrows_timeorder
	// or *.arrows_WAL files)? If so, quit.
	// Otherwise, begin to watch for file events and sort or shuffle as needed.
	pattern := filepath.Join(directory, "*.arrows_timeorder")
	matches1, err := filepath.Glob(pattern)
	if err != nil {
		log.Print(err)
		return
	}
	pattern = filepath.Join(directory, "*.arrows_WAL")
	matches2, err := filepath.Glob(pattern)
	if err != nil {
		log.Print(err)
		return
	}
	if len(matches1) == 0 && len(matches2) == 0 {
		log.Printf("✅ Found no files that need FARMER. Quitting")
		return
	}

	log.Printf("No more existing files to organize. Watching for new files.")
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// We only care about file creations
			if event.Has(fsnotify.Create) {

				// Ignore most files, except "COMPLETE" or "*.arrows_timeorder" files
				_, filename := filepath.Split(event.Name)
				if strings.HasSuffix(filename, ".arrows_timeorder") {
					// Careful! It's possible that a file like this was caught by the watcher, THEN
					// found at the start of this goroutine by the filepath.Glob() call and handled at that point.
					// Thus, it's possible that it no longer exists. It's gone, ignore it gracefully.
					if _, err := os.Stat(event.Name); err == nil {
						log.Printf("Detected new ready file: %s", event.Name)
						sortIPCFile(event.Name, db)
					}

				} else if filename == "COMPLETE" {
					// First, stop the watcher, to conserve OS resources. We know its work is done.
					watcher.Remove(directory)

					// Next, ensure that all the timeorder files have been processed and removed.
					inputPattern := fmt.Sprintf("%s/*.arrows_timeorder", directory)
					files, _ := filepath.Glob(inputPattern)
					for _, file := range files {
						log.Printf("Detected new ready file: %s", file)
						sortIPCFile(file, db)
					}

					// Directory is ready for shuffling now.
					shuffleDirectory(directory)
					return
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func sortIPCFile(streamFile string, db *sql.DB) {
	// Convert file.suffix_timeorder to file.suffix (trim off the sub-suffix)
	outputFile := strings.TrimSuffix(streamFile, "_timeorder")

	// 3. Execute the Zero-Copy Sort and Export
	// read_arrow() parses the un-footered stream.
	// DuckDB sorts it out-of-core (using your SSD if it exceeds RAM).
	// COPY ... TO exports the result natively.

	escapeSQLString := func(s string) string {
		return strings.ReplaceAll(s, "'", "''")
	}
	query := fmt.Sprintf(`
		COPY (
			SELECT * 
			FROM read_arrow('%s')
			ORDER BY channel_number ASC, subframecount ASC
		) TO '%s' (FORMAT ARROWS);
	`, escapeSQLString(streamFile), escapeSQLString(outputFile))

	_, err := db.Exec(query)
	if err != nil {
		log.Println("Failed to process query: ", err)
		return
	}

	err = os.Remove(streamFile)
	if err != nil {
		log.Printf("Could not remove %s\n", streamFile)
	}

	log.Printf("✅ Data successfully sorted and saved to %s\n", outputFile)
}

// ChannelUnshuffler manages N open Feather writers, one for each channel
type ChannelUnshuffler struct {
	outputDir   string
	prefix      string
	pool        memory.Allocator
	schema      *arrow.Schema
	writers     map[int64]*ipc.FileWriter
	fileHandles map[int64]*os.File
}

func NewChannelUnshuffler(outputDir, prefix string) *ChannelUnshuffler {
	return &ChannelUnshuffler{
		outputDir:   outputDir,
		prefix:      prefix,
		pool:        memory.NewGoAllocator(),
		writers:     make(map[int64]*ipc.FileWriter),
		fileHandles: make(map[int64]*os.File),
	}
}

// GetWriter fetches an existing Feather writer or creates a new one for a channel.
// Can call this without knowing whether we've already opened a writer for `channelID` already.
func (u *ChannelUnshuffler) GetWriter(channelID int64) (*ipc.FileWriter, error) {
	if w, exists := u.writers[channelID]; exists {
		return w, nil
	}

	// Create output folder if it doesn't exist
	if err := os.MkdirAll(u.outputDir, 0755); err != nil {
		return nil, err
	}

	// Open destination Feather file
	filePath := filepath.Join(u.outputDir, fmt.Sprintf("%spulses_chan%d.arrow", u.prefix, channelID))
	f, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file for channel %d: %w", channelID, err)
	}

	// Initialize the Feather (IPC File) writer with the dataset schema
	w, err := ipc.NewFileWriter(f, ipc.WithAllocator(u.pool), ipc.WithSchema(u.schema))
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to create Feather writer for channel %d: %w", channelID, err)
	}

	u.fileHandles[channelID] = f
	u.writers[channelID] = w
	return w, nil
}

// ProcessBatch scans a record batch and slices contiguous channel blocks (Zero-Copy)
func (u *ChannelUnshuffler) ProcessBatch(rec arrow.RecordBatch) error {
	if u.schema == nil {
		u.schema = rec.Schema()
	}

	// Locate the channel_number column
	colIdx := rec.Schema().FieldIndices("channel_number")
	if len(colIdx) == 0 {
		return fmt.Errorf("column 'channel_number' not found in schema")
	}

	chCol := rec.Column(colIdx[0])
	numRows := rec.NumRows()
	if numRows == 0 {
		return nil
	}

	start := int64(0)
	for i := range numRows {
		currentCh := getChannelValue(chCol, i)

		// Find where the channel block ends (either channel changes or end of batch)
		if i == numRows-1 || getChannelValue(chCol, i+1) != currentCh {
			end := i + 1

			// ZERO-COPY SLICE: Creates a window over memory without duplicating data
			slice := rec.NewSlice(start, end)

			writer, err := u.GetWriter(currentCh)
			if err != nil {
				slice.Release()
				return err
			}

			// Write the sliced batch to the channel's Feather file
			if err := writer.Write(slice); err != nil {
				slice.Release()
				return err
			}

			slice.Release() // Release slice memory reference
			start = end
		}
	}
	return nil
}

// Close finalized all Feather files by appending the Arrow metadata footer
func (u *ChannelUnshuffler) Close() {
	log.Printf("Finalizing %d channel Feather files...", len(u.writers))
	for ch, w := range u.writers {
		// CRITICAL: Close writes the Feather footer to disk
		if err := w.Close(); err != nil {
			log.Printf("Error writing footer for channel %d: %v", ch, err)
		}
		if f, ok := u.fileHandles[ch]; ok {
			f.Close()
		}
	}
}

// Helper to safely extract channel integer value regardless of bit-width
func getChannelValue(col arrow.Array, idx int64) int64 {
	switch c := col.(type) {
	case *array.Int64:
		return c.Value(int(idx))
	case *array.Int32:
		return int64(c.Value(int(idx)))
	case *array.Int16:
		return int64(c.Value(int(idx)))
	default:
		panic(fmt.Sprintf("unsupported channel_number array type: %T", col))
	}
}

// Declare the regex globally so it need not be compiled multiple times.
// This lets us strip a trailing _1234.suffix from a filename, where "1234" is any
// four digits, and "suffix" is any string containing no periods.
var patternForNumberedFiles = regexp.MustCompile(`_\d{4}\.[^.]+$`)

func shuffleDirectory(dir string) {
	inputPattern := fmt.Sprintf("%s/*_????.arrows", dir)
	outputDir := dir
	log.Printf("Directory %s is marked COMPLETE\n", dir)
	log.Printf("Starting to unshuffle all files %s\n", inputPattern)

	// 1. Find and chronologically sort all input stream files
	files, err := filepath.Glob(inputPattern)
	if err != nil || len(files) == 0 {
		log.Printf("No input stream files found matching pattern: %s", inputPattern)
		return
	}
	sort.Strings(files) // Ensures {prefix}_0000,  {prefix}_0001, {prefix}__0002, etc. run in order
	f := files[0]
	firstarrow := path.Base(f)
	prefix := ""
	if locate := patternForNumberedFiles.FindStringIndex(firstarrow); locate != nil {
		prefix = firstarrow[:locate[0]]
	}

	log.Printf("Found %d stream files to process; prefix='%s'.", len(files), prefix)

	unshuffler := NewChannelUnshuffler(outputDir, prefix)
	defer unshuffler.Close() // Ensures all Feather footers are written when main exits!

	pool := memory.NewGoAllocator()

	// 2. Loop through each mixed-channel, time-limited .arrows stream file
	for _, file := range files {
		// Run each iteration as an anonymous function, to simplify
		// deferred closing of opened file and release of ipc.NewReader
		err := func(filename string) error {
			log.Printf("Unshuffling %s...", file)

			f, err := os.Open(file)
			if err != nil {
				return fmt.Errorf("❌ Failed to open %s: %v", file, err)
			}
			defer f.Close()

			rdr, err := ipc.NewReader(f, ipc.WithAllocator(pool))
			if err != nil {
				return fmt.Errorf("❌ Failed to create IPC reader for %s: %v", file, err)
			}
			defer rdr.Release()

			// 3. Read stream batches sequentially
			for rdr.Next() {
				rec := rdr.RecordBatch()

				if err := unshuffler.ProcessBatch(rec); err != nil {
					return fmt.Errorf("❌ Failed to unshuffle batch: %v", err)
				}
			}
			return nil
		}(file)

		// If any one file fails, abort the whole directory
		if err != nil {
			log.Printf("%v", err)
			return
		}
	}

	// 3. If this point is reached successfully, it is safe to delete the input files
	log.Printf("✅ Unshuffling %s complete! All channels saved as zero-copy Feather files.\n", dir)
	os.Remove(filepath.Join(dir, "COMPLETE"))
	for _, file := range files {
		err = os.Remove(file)
		if err != nil {
			log.Printf("❌ Could not remove %s\n", file)
		}
	}

}
