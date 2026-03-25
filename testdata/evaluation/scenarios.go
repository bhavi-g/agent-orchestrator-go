package evaluation

// Scenario describes a labeled evaluation case.
type Scenario struct {
	// Name is the folder name under testdata/evaluation/.
	Name string

	// Description explains what the logs contain.
	Description string

	// ExpectSuccess is true if the pipeline should complete without error.
	ExpectSuccess bool

	// ExpectConfidence is the expected confidence_level from the analyzer.
	ExpectConfidence string // "Low", "Medium", "High"

	// MinErrors is the minimum number of error/warning lines expected.
	MinErrors int

	// ExpectNoIssues is true when no errors/warnings are expected.
	ExpectNoIssues bool

	// ExpectFiles lists filenames that should appear in supporting_evidence.
	ExpectFiles []string

	// ExpectPatterns lists substrings that should appear in the output
	// (error_summary or suspected_root_cause).
	ExpectPatterns []string
}

// Scenarios returns the full evaluation dataset.
func Scenarios() []Scenario {
	return []Scenario{
		{
			Name:             "01-clean",
			Description:      "No errors or warnings — healthy system logs only",
			ExpectSuccess:    true,
			ExpectConfidence: "High",
			ExpectNoIssues:   true,
			ExpectPatterns:   []string{"No errors"},
		},
		{
			Name:             "02-single-error",
			Description:      "Single SMTP error in otherwise healthy logs",
			ExpectSuccess:    true,
			ExpectConfidence: "Low",
			MinErrors:        1,
			ExpectFiles:      []string{"app.log"},
			ExpectPatterns:   []string{"SMTP", "email"},
		},
		{
			Name:             "03-repeated-errors",
			Description:      "Same connection-refused error repeated 5 times (triggers High)",
			ExpectSuccess:    true,
			ExpectConfidence: "High",
			MinErrors:        5,
			ExpectFiles:      []string{"gateway.log"},
			ExpectPatterns:   []string{"Connection refused", "payment"},
		},
		{
			Name:             "04-panic-crash",
			Description:      "OOM errors followed by a FATAL panic",
			ExpectSuccess:    true,
			ExpectConfidence: "High",
			MinErrors:        3,
			ExpectFiles:      []string{"server.log"},
			ExpectPatterns:   []string{"panic", "memory"},
		},
		{
			Name:             "05-multi-file-cascade",
			Description:      "Auth failure cascading through API and frontend (3 log files)",
			ExpectSuccess:    true,
			ExpectConfidence: "High",
			MinErrors:        5,
			ExpectFiles:      []string{"auth.log", "api.log", "frontend.log"},
			ExpectPatterns:   []string{"auth", "500", "503"},
		},
		{
			Name:             "06-warnings-only",
			Description:      "Only WARN-level entries: CPU, disk, network",
			ExpectSuccess:    true,
			ExpectConfidence: "Medium",
			MinErrors:        1,
			ExpectFiles:      []string{"monitor.log"},
			ExpectPatterns:   []string{"warn"},
		},
		{
			Name:             "07-mixed-severity",
			Description:      "ERRORs + WARNs + FATAL panic across app and worker",
			ExpectSuccess:    true,
			ExpectConfidence: "High",
			MinErrors:        5,
			ExpectFiles:      []string{"app.log"},
			ExpectPatterns:   []string{"panic"},
		},
		{
			Name:             "08-database-timeout",
			Description:      "Database query timeouts across orders and users services",
			ExpectSuccess:    true,
			ExpectConfidence: "Medium",
			MinErrors:        5,
			ExpectFiles:      []string{"orders.log", "users.log"},
			ExpectPatterns:   []string{"timeout", "Database"},
		},
	}
}
