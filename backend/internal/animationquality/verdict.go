package animationquality

// Legacy manual verdicts are historical only. Quality is the single source for
// animation conclusions; this mapping is not model identity or health evidence.
func QualityVerdict(label string) string {
	switch label {
	case "normal":
		return "passed"
	case "suspicious":
		return "not_passed"
	default:
		return "inconclusive"
	}
}

type AnnotationRecord struct {
	SourceSHA256 string
	PromptSHA256 string
	Annotation   Annotation
}
