package handlers

import "errors"

// Sentinel errors for HTTP request validation.
var (
	ErrResultsRequired       = errors.New("'results' array is required in the body")
	ErrResultsEmpty          = errors.New("'results' array is empty")
	ErrFileNameRequired      = errors.New("'file_name' attribute is required for all results")
	ErrDuplicateFileNames    = errors.New("duplicated file names in 'results'")
	ErrContentBase64Required = errors.New("'content_base64' attribute is required")
	ErrNoFilesProvided       = errors.New("no files provided in 'files[]' field")

	// tar.gz archive validation errors.
	ErrArchiveEmpty         = errors.New("archive contains no files")
	ErrArchiveTooManyFiles  = errors.New("archive exceeds maximum file count")
	ErrArchiveDecompBomb    = errors.New("decompressed archive exceeds size limit")
	ErrArchiveNestedPath    = errors.New("archive entry contains nested path")
	ErrArchiveInvalidEntry  = errors.New("archive entry is not a regular file")
	ErrArchiveDuplicateFile = errors.New("archive contains duplicate file names")

	// report_id validation errors.
	ErrReportIDRequired = errors.New("report_id is required")
	ErrReportIDInvalid  = errors.New("report_id must be 'latest' or a positive integer")

	// ticket_url validation errors.
	ErrTicketURLInvalidScheme = errors.New("ticket_url must use http or https scheme")

	// errUnsupportedContentType is returned by parseResultsBody when the
	// Content-Type header is not application/json, multipart/form-data, or application/gzip.
	errUnsupportedContentType = errors.New("unsupported Content-Type")
)

// Package-level limits for tar.gz extraction (vars for testability).
//
//nolint:gochecknoglobals // overridden in tests to avoid creating huge archives
var (
	maxDecompressedBytes int64 = 1 << 30 // 1 GB
	maxArchiveFileCount        = 5000
)
