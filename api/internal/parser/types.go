package parser

// Result holds the fully parsed data from one Allure result JSON file.
type Result struct {
	Name          string
	FullName      string
	HistoryID     string
	Status        string
	StatusMessage string // from statusDetails.message
	StatusTrace   string // from statusDetails.trace
	Description   string
	StartMs       int64
	StopMs        int64
	DurationMs    int64
	Flaky         bool
	Retries       int
	Labels        []Label
	Parameters    []Parameter
	Steps         []Step
	Attachments   []Attachment
	// Attempts holds one entry per execution attempt of this test within the
	// build, in attempt order. The Result's own Status/StatusMessage describe
	// the FINAL attempt only; Attempts is what lets a consumer tell a test that
	// failed the same way three times from one that failed differently each
	// time. Empty when the report format carries no per-attempt detail.
	Attempts []Attempt
}

// Attempt holds the outcome of a single execution attempt of a test.
//
// Status is recorded verbatim from the source report rather than normalized to
// the Allure status vocabulary: a Playwright attempt reports "timedOut" or
// "interrupted", and collapsing those into "broken" would discard exactly the
// distinction a retry comparison exists to surface.
type Attempt struct {
	// Index is the zero-based attempt number: 0 is the first run, 1 the first
	// retry, and so on.
	Index int
	// Status is the attempt's outcome as the source report spelled it.
	Status string
	// StatusMessage is the attempt's error text. Empty for a passing attempt.
	StatusMessage string
}

// Label holds an Allure label (e.g. suite, feature, severity, owner, epic, tag).
type Label struct {
	Name  string
	Value string
}

// Parameter holds a test parameter (name/value pair for parameterized tests).
type Parameter struct {
	Name  string
	Value string
}

// Step holds one test step, potentially containing sub-steps and attachments.
type Step struct {
	Name          string
	Status        string
	StatusMessage string
	DurationMs    int64
	Order         int
	Steps         []Step
	Attachments   []Attachment
}

// Attachment holds file attachment metadata (actual files remain on filesystem/S3).
type Attachment struct {
	Name     string
	Source   string // filename in storage
	MimeType string
	Size     int64 // populated by ResolveAttachments; 0 when unknown
}
