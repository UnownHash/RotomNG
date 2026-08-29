package ginutil

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// gzipMinLength is the smallest response worth compressing. Below roughly a
// kilobyte the gzip header and trailer cost more than the deflate stream saves,
// so the many small `{"status":"ok"}` action replies are written through
// untouched.
const gzipMinLength = 1024

// gzipExcludedExtensions lists the static assets that are already compressed
// on disk. Re-deflating them burns CPU for a fraction of a percent, and in the
// case of the fonts can make the response marginally larger. The middleware's
// own defaults cover .png/.gif/.jpeg/.jpg; the rest are ours.
var gzipExcludedExtensions = []string{
	".png", ".gif", ".jpeg", ".jpg", ".webp", ".avif",
	".ico", ".woff", ".woff2", ".zip", ".gz",
}

// gzipExcludedPathRegexps skips endpoints whose bodies are already a compressed
// container. Logcat replies are a zip built in memory and served with an
// explicit Content-Length, so there is nothing left for deflate to find.
var gzipExcludedPathRegexps = []string{
	`/action/logcat$`,
}

// GzipMiddleware compresses responses for clients that advertise gzip support.
//
// This matters far more than it looks: /api/status serialises every device,
// worker, and controller on every poll, and the bulk of that JSON is the same
// field names repeated once per record -- exactly the redundancy deflate is
// built for, and the saving grows with the number of connections reported.
// Once the response outgrows the poll interval, requests start overlapping
// and never catch up; compressing it keeps a poll inside its own interval.
//
// The middleware also covers the embedded UI, so the JS and CSS bundles stop
// going out uncompressed.
//
// Two things are deliberately left alone. WebSocket upgrades are skipped by the
// middleware itself (it refuses any request carrying Connection: Upgrade), so
// device and controller sockets are unaffected. Responses that already carry a
// Content-Encoding are passed through rather than compressed twice.
func GzipMiddleware() gin.HandlerFunc {
	return gzip.Gzip(
		gzip.DefaultCompression,
		gzip.WithExcludedExtensions(gzipExcludedExtensions),
		gzip.WithExcludedPathsRegexs(gzipExcludedPathRegexps),
		gzip.WithMinLength(gzipMinLength),
	)
}
