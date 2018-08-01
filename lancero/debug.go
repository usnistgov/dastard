// +build debug

package lancero

func debug(fmt string, args ...interface{}) {
	fmt.Printf(fmt, args...)
}
