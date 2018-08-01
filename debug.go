// +build debug

package dastard

func debug(fmt string, args ...interface{}) {
	fmt.Printf(fmt, args...)
}
