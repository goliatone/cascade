//go:build !unix

package automation

func isInterruptedSystemCall(error) bool {
	return false
}
