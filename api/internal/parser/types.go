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
	Labels        []Label
	Parameters    []Parameter
	Steps         []Step
	Attachments   []Attachment
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
}
