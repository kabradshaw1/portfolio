package lint

// LineFromOffset returns the 1-based line number containing the byte at offset
// in src. Out-of-range offsets clamp to the last line.
func LineFromOffset(src []byte, offset int) int {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	line := 1
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}

// Lint parses each file in paths and runs the supplied rules over each
// statement. Returns the aggregated violations. A parse error stops processing
// of that file (and is returned as the error) but other files continue.
func Lint(paths []string, rules []Rule) ([]Violation, error) {
	var all []Violation
	for _, path := range paths {
		ctx, err := parseFile(path)
		if err != nil {
			return all, err
		}
		ctx.Ignores = parseIgnoreDirectives(ctx.Source, ctx.Statements)
		for i, stmt := range ctx.Statements {
			ctx.Index = i
			for _, rule := range rules {
				if isIgnored(ctx, stmt, rule.ID()) {
					continue
				}
				all = append(all, rule.Check(stmt, ctx)...)
			}
		}
	}
	return all, nil
}
