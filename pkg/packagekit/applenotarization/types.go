package applenotarization

import "time"

// notarizationResponse is the response from altool. It probably only
// have NotarizationInfo or NotarizationUpload populated.
type notarizationResponse struct {
	ProductErrors      []productError   `plist:"product-errors"`
	NotarizationInfo   notarizationInfo `plist:"notarization-info"`
	NotarizationUpload notarizationInfo `plist:"notarization-upload"`
}

type notarizationInfo struct {
	Date          time.Time `plist:"Date"`
	LogFileURL    string    `plist:"LogFileURL"`
	RequestUUID   string    `plist:"RequestUUID"`
	Status        string    `plist:"Status"`
	StatusCode    int       `plist:"Status Code"`
	StatusMessage string    `plist:"Status Message"`
}

// this is eventually used by callers in other repos
//nolint:deadcode
type notarizationUpload struct {
	ProductErrors []productError `plist:"product-errors"`
}

type productError struct {
	Code    int    `plist:"code"`
	Message string `plist:"message"`
}
