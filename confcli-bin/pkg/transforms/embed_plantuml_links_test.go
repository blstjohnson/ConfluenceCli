package transforms

import "testing"

func TestEmbedPlantUMLLinks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain link",
			in:   `See [diagram](../Diagrams/foo.puml) for details.`,
			want: `See ![diagram](../Diagrams/foo.puml) for details.`,
		},
		{
			name: "full bold wrap stripped",
			in:   `**[prc.puml](../Diagrams/prc.puml)**`,
			want: `![prc.puml](../Diagrams/prc.puml)`,
		},
		{
			name: "bold inside link text stripped",
			in:   `[**bold text**](path/file.puml)`,
			want: `![bold text](path/file.puml)`,
		},
		{
			name: "link inside wider bold run leaves bold intact",
			in:   `**See [diagram](x.puml) for the flow**`,
			want: `**See ![diagram](x.puml) for the flow**`,
		},
		{
			name: "uppercase extension",
			in:   `[file](FOO.PUML)`,
			want: `![file](FOO.PUML)`,
		},
		{
			name: "mixed-case .plantuml extension",
			in:   `[file](dir/Foo.PlantUML)`,
			want: `![file](dir/Foo.PlantUML)`,
		},
		{
			name: "multiple links on one line",
			in:   `[a](a.puml) and [b](b.plantuml) and [c](c.md)`,
			want: `![a](a.puml) and ![b](b.plantuml) and [c](c.md)`,
		},
		{
			name: "non-puml link untouched",
			in:   `[readme](readme.md)`,
			want: `[readme](readme.md)`,
		},
		{
			name: "false-positive .puml in path segment",
			in:   `[x](dir.puml/file.html)`,
			want: `[x](dir.puml/file.html)`,
		},
		{
			name: "already an image left alone",
			in:   `![diagram](foo.puml)`,
			want: `![diagram](foo.puml)`,
		},
		{
			name: "empty link text",
			in:   `[](foo.puml)`,
			want: `![](foo.puml)`,
		},
		{
			name: "full bold wrap with bold inside text",
			in:   `**[**name**](path.puml)**`,
			want: `![name](path.puml)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewEmbedPlantUMLLinks()
			ctx := &TransformContext{PostContent: tc.in}
			if err := tr.Apply(ctx); err != nil {
				t.Fatal(err)
			}
			if ctx.PostContent != tc.want {
				t.Errorf("\n in:   %q\n want: %q\n got:  %q", tc.in, tc.want, ctx.PostContent)
			}
		})
	}
}

func TestEmbedPlantUMLLinks_SkipsCodeBlocks(t *testing.T) {
	in := "Outside [a](a.puml)\n" +
		"```\n" +
		"Inside [b](b.puml) — should not change\n" +
		"```\n" +
		"After [c](c.puml)\n"
	want := "Outside ![a](a.puml)\n" +
		"```\n" +
		"Inside [b](b.puml) — should not change\n" +
		"```\n" +
		"After ![c](c.puml)\n"

	tr := NewEmbedPlantUMLLinks()
	ctx := &TransformContext{PostContent: in}
	if err := tr.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PostContent != want {
		t.Errorf("\n want: %q\n got:  %q", want, ctx.PostContent)
	}
}

func TestEmbedPlantUMLLinks_TildeFences(t *testing.T) {
	in := "~~~\n[a](a.puml)\n~~~\n[b](b.puml)\n"
	want := "~~~\n[a](a.puml)\n~~~\n![b](b.puml)\n"

	tr := NewEmbedPlantUMLLinks()
	ctx := &TransformContext{PostContent: in}
	if err := tr.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PostContent != want {
		t.Errorf("\n want: %q\n got:  %q", want, ctx.PostContent)
	}
}
