package api

// SetMaxUploadBodyBytes lowers the CreateFile body guard for a test and returns
// a function restoring the previous value.
func SetMaxUploadBodyBytes(n int64) func() {
	old := maxUploadBodyBytes
	maxUploadBodyBytes = n
	return func() { maxUploadBodyBytes = old }
}
