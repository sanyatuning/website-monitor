package inspect

import (
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/sanyatuning/website-monitor/internal/config"
)

// Inspector monitors an url every polling interval, and sends reports over `reportc` channel
type Inspector struct {
	ticker    *time.Ticker     // periodic ticker of periodicity `PollingInterval`
	url       string           // current URLs
	reportc   chan *Report     // channel used to report metrics
	collector *colly.Collector // colly collector which sends and traces HTTP requests
}

// Report collects useful metrics from a single HTTP request made by an Inspector
type Report struct {
	Url               string
	PollingInterval   time.Duration
	StatusCode        int
	ConnectDuration   time.Duration
	FirstByteDuration time.Duration
}

// NewInspector initializes an Inspector and returns the channel over which it communicates reports
func NewInspector(url string, PollingInterval time.Duration) <-chan *Report {
	// number of reports to keep track of (we keep reports as old as `LongStatsHistoryInterval`)
	maxNumOfReports := int(config.LongStatsHistoryInterval / PollingInterval)
	reportc := make(chan *Report, maxNumOfReports)

	// define collector
	collector := newTraceCollector()

	// set timeout equal to PollingInterval
	collector.SetRequestTimeout(time.Second * 10)

	// Set response handler
	collector.OnResponse(func(resp *colly.Response) {
		if resp.Trace == nil {
			//TODO: check if we ever reach here
			errReport := &Report{
				Url:               url,
				PollingInterval:   PollingInterval,
				StatusCode:        resp.StatusCode,
				ConnectDuration:   -1,
				FirstByteDuration: -1,
			}

			// send error report to metrics
			reportc <- errReport
		}
		// create report from trace
		report := &Report{
			Url:               url,
			PollingInterval:   PollingInterval,
			StatusCode:        resp.StatusCode,
			ConnectDuration:   resp.Trace.ConnectDuration,
			FirstByteDuration: resp.Trace.FirstByteDuration,
		}

		// send report over to metrics for further analytics
		reportc <- report
	})

	// Set error handler
	// By default, Colly parses only successful HTTP responses. Set ParseHTTPErrorResponse
	// to true to enable parsing status codes other than 2xx.
	// For simplicity we'll consider a website not available if the HTTP response is not successful
	collector.OnError(func(resp *colly.Response, err error) {
		// log.Println("Request URL:", resp.Request.URL, "failed with response:", resp, "\nError:", err)
		errReport := &Report{
			Url:               url,
			PollingInterval:   PollingInterval,
			StatusCode:        resp.StatusCode,
			ConnectDuration:   -1,
			FirstByteDuration: -1,
		}

		// send error report to metrics
		reportc <- errReport
	})

	// init new inspector
	inspector := &Inspector{
		ticker:    time.NewTicker(PollingInterval),
		reportc:   reportc,
		url:       url,
		collector: collector,
	}

	// start monitoring
	go inspector.startInspecting()
	return reportc
}

// newTraceCollector creates a new `colly` collector which traces http requests
func newTraceCollector() *colly.Collector {
	collector := colly.NewCollector(colly.TraceHTTP(), colly.AllowURLRevisit(), colly.ParseHTTPErrorResponse())
	return collector
}

// startInspecting start inspection loop of the url every `PollingInterval`
func (inspector *Inspector) startInspecting() {
	for range inspector.ticker.C {
		// When the ticker fires, inspect url
		go inspector.collector.Visit(inspector.url)
	}
}
