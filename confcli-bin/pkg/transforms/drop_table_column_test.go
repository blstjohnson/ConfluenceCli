package transforms

import "testing"

func TestDropTableColumnByIndex(t *testing.T) {
	dt, err := NewDropTableColumn([]int{1}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<table><tr><th>A</th><th>B</th><th>C</th></tr>` +
			`<tr><td>1</td><td>2</td><td>3</td></tr></table>`,
	}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// Column B (index 1) should be removed
	if contains(ctx.PreContent, "<th>B</th>") {
		t.Errorf("column B header should be removed, got %q", ctx.PreContent)
	}
	if contains(ctx.PreContent, "<td>2</td>") {
		t.Errorf("column B data should be removed, got %q", ctx.PreContent)
	}
	// Columns A and C should remain
	if !contains(ctx.PreContent, "A") {
		t.Errorf("column A should remain, got %q", ctx.PreContent)
	}
	if !contains(ctx.PreContent, "C") {
		t.Errorf("column C should remain, got %q", ctx.PreContent)
	}
}

func TestDropTableColumnByHeaderPattern(t *testing.T) {
	dt, err := NewDropTableColumn(nil, []string{`(?i)параметры`})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<table><tr><th>Name</th><th>Параметры сообщения</th><th>Value</th></tr>` +
			`<tr><td>a</td><td>b</td><td>c</td></tr></table>`,
	}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if contains(ctx.PreContent, "Параметры") {
		t.Errorf("matched column header should be removed, got %q", ctx.PreContent)
	}
	if contains(ctx.PreContent, "<td>b</td>") {
		t.Errorf("matched column data should be removed, got %q", ctx.PreContent)
	}
	if !contains(ctx.PreContent, "Name") || !contains(ctx.PreContent, "Value") {
		t.Errorf("non-matched columns should remain, got %q", ctx.PreContent)
	}
}

func TestDropTableColumnMultipleIndices(t *testing.T) {
	dt, err := NewDropTableColumn([]int{0, 2}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<table><tr><th>A</th><th>B</th><th>C</th></tr>` +
			`<tr><td>1</td><td>2</td><td>3</td></tr></table>`,
	}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// Only column B should remain
	if !contains(ctx.PreContent, "B") {
		t.Errorf("column B should remain, got %q", ctx.PreContent)
	}
	if contains(ctx.PreContent, "<th>A</th>") || contains(ctx.PreContent, "<th>C</th>") {
		t.Errorf("columns A and C should be removed, got %q", ctx.PreContent)
	}
}

func TestDropTableColumnWithThead(t *testing.T) {
	dt, err := NewDropTableColumn(nil, []string{`^Remove$`})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<table><thead><tr><th>Keep</th><th>Remove</th></tr></thead>` +
			`<tbody><tr><td>yes</td><td>no</td></tr></tbody></table>`,
	}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if contains(ctx.PreContent, "Remove") {
		t.Errorf("Remove column should be dropped, got %q", ctx.PreContent)
	}
	if contains(ctx.PreContent, "<td>no</td>") {
		t.Errorf("Remove column data should be dropped, got %q", ctx.PreContent)
	}
	if !contains(ctx.PreContent, "Keep") || !contains(ctx.PreContent, "yes") {
		t.Errorf("Keep column should remain, got %q", ctx.PreContent)
	}
}

func TestDropTableColumnEmptyContent(t *testing.T) {
	dt, err := NewDropTableColumn([]int{0}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{PreContent: ""}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != "" {
		t.Errorf("expected empty content, got %q", ctx.PreContent)
	}
}

func TestDropTableColumnNoTables(t *testing.T) {
	dt, err := NewDropTableColumn([]int{0}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{PreContent: "<p>No tables here</p>"}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !contains(ctx.PreContent, "No tables here") {
		t.Errorf("non-table content should be preserved, got %q", ctx.PreContent)
	}
}

func TestDropTableColumnInvalidPattern(t *testing.T) {
	_, err := NewDropTableColumn(nil, []string{"[invalid"})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestDropTableColumnName(t *testing.T) {
	dt, err := NewDropTableColumn(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dt.Name() != "drop/table_column" {
		t.Errorf("expected 'drop/table_column', got %q", dt.Name())
	}
}

func TestDropTableColumnIndexAndPattern(t *testing.T) {
	// Both index and pattern can be used together
	dt, err := NewDropTableColumn([]int{0}, []string{`^C$`})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<table><tr><th>A</th><th>B</th><th>C</th></tr>` +
			`<tr><td>1</td><td>2</td><td>3</td></tr></table>`,
	}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// Column A (index 0) and C (pattern match) should be removed
	if !contains(ctx.PreContent, "B") {
		t.Errorf("column B should remain, got %q", ctx.PreContent)
	}
	if contains(ctx.PreContent, "<th>A</th>") {
		t.Errorf("column A should be removed by index, got %q", ctx.PreContent)
	}
	if contains(ctx.PreContent, "<th>C</th>") {
		t.Errorf("column C should be removed by pattern, got %q", ctx.PreContent)
	}
}

func TestDropTableColumnMultipleTables(t *testing.T) {
	dt, err := NewDropTableColumn([]int{1}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<table><tr><th>X</th><th>Y</th></tr><tr><td>1</td><td>2</td></tr></table>` +
			`<p>between</p>` +
			`<table><tr><th>P</th><th>Q</th></tr><tr><td>3</td><td>4</td></tr></table>`,
	}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// Column index 1 should be dropped from both tables
	if contains(ctx.PreContent, "Y") || contains(ctx.PreContent, "Q") {
		t.Errorf("second column should be removed from both tables, got %q", ctx.PreContent)
	}
	if !contains(ctx.PreContent, "X") || !contains(ctx.PreContent, "P") {
		t.Errorf("first column should remain in both tables, got %q", ctx.PreContent)
	}
	if !contains(ctx.PreContent, "between") {
		t.Errorf("non-table content should be preserved, got %q", ctx.PreContent)
	}
}

func TestDropTableColumnPostContentUntouched(t *testing.T) {
	dt, err := NewDropTableColumn([]int{0}, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent:  `<table><tr><th>A</th><th>B</th></tr></table>`,
		PostContent: "untouched",
	}
	if err := dt.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PostContent != "untouched" {
		t.Errorf("PostContent should be untouched, got %q", ctx.PostContent)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
