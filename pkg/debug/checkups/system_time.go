package checkups

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type systemTime struct {
	summary string
	status  Status
}

func (st *systemTime) Name() string {
	return "System Time"
}

func (st *systemTime) Run(_ context.Context, extraFh io.Writer) error {
	var (
		urls = []string{
			"https://k2control.kolide.com/version",
			"https://developers.google.com/time/",
			"http://sha256timestamp.ws.symantec.com/",
		}
	)

	for _, url := range urls {
		serverTime, err := getTimeFromDateHeader(url)
		if err != nil {
			fmt.Fprintf(extraFh, "error from url %s: %v\n", url, err)
			continue
		}

		serverTime = serverTime.UTC()

		fmt.Fprintf(extraFh, "pulled date header from %s: %v\n", url, serverTime)

		systemTime := time.Now().UTC()

		fmt.Fprintf(extraFh, "system time: %v\n", systemTime)

		diff := systemTime.Sub(serverTime)
		if diff < 0 {
			diff = -diff
		}

		maxDiff := 5 * time.Minute
		if diff > maxDiff {
			st.summary = fmt.Sprintf("system time off by more than %f minutes when compared to server date header, delta = %f minutes", maxDiff.Minutes(), diff.Minutes())
			st.status = Warning
			return nil
		}

		// all urls errored
		st.summary = fmt.Sprintf("system time is within %f minutes of server date header, delta = %f minutes", maxDiff.Minutes(), diff.Minutes())
		st.status = Passing
		return nil
	}

	// if we made it here, we never got valid server time
	st.summary = "could not get valid server time"
	st.status = Erroring
	return nil
}

func getTimeFromDateHeader(url string) (time.Time, error) {
	// Make an HTTP GET request to the specified URL.
	resp, err := http.Get(url)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	// Parse the "Date" header from the response.
	dateHeader := resp.Header.Get("Date")
	if dateHeader == "" {
		return time.Time{}, fmt.Errorf("Date header not found in response")
	}

	// Parse the date string from the header.
	parsedTime, err := time.Parse(time.RFC1123, dateHeader)
	if err != nil {
		return time.Time{}, err
	}

	return parsedTime, nil
}

func (st *systemTime) ExtraFileName() string {
	return "systemTime.log"
}

func (st *systemTime) Status() Status {
	return st.status
}

func (st *systemTime) Summary() string {
	return st.summary
}

func (st *systemTime) Data() any {
	return nil
}
