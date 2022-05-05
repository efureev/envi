package envi

import (
	"testing"
)

func TestNewBlock(t *testing.T) {

	block := NewBlock(`app`)

	if block.Prefix != `APP` {
		t.Fatalf("should be `APP`")
	}

	if block.Comment != `` {
		t.Fatalf("should be ``")
	}
	block.SetComment(`Comment`)

	if block.Comment != `Comment` {
		t.Fatalf("should be `Comment`")
	}

	if len(block.Rows) != 0 {
		t.Fatalf("should be `0`")
	}

	if block.SetPrefix(`app--ser222-_-3`).Prefix != `APP_SER222_3` {
		t.Fatalf("should be `APP_SER222_3` instead `%v`", block.Prefix)
	}
}

func TestBlock_AddRow(t *testing.T) {
	block := NewBlock(`app`)
	block.AddRow(`session`, `.example.com`)

	if len(block.Rows) != 1 {
		t.Fatalf("should be `1`")
	}

	r := block.GetRow(`session`)
	if r.Value != `.example.com` {
		t.Fatalf("should be `.example.com`")
	}
}

func TestBlock_AddRows(t *testing.T) {
	block := NewBlock(`app`)
	block.AddRows(
		NewRow(`session`, `.example.com`),
		NewRow(`url`, `https://example.com`).SetComment(`Application URL`),
	)

	if len(block.Rows) != 2 {
		t.Fatalf("should be `2`")
	}

	r := block.GetRow(`session`)
	if r.Value != `.example.com` {
		t.Fatalf("should be `.example.com`")
	}
	r2 := block.GetRow(`url`)
	if r2.Value != `https://example.com` {
		t.Fatalf("should be `https://example.com`")
	}
	if r2.Comment != `Application URL` {
		t.Fatalf("should be `Application URL`")
	}

	block.RemoveRow(`url`)
	if len(block.Rows) != 1 {
		t.Fatalf("should be `1`")
	}
	rNil := block.GetRow(`url`)
	if rNil != nil {
		t.Fatalf("should be `nil`")
	}

	block.RemoveRow(`session`)
	if len(block.Rows) != 0 {
		t.Fatalf("should be `0`")
	}
	rNil = block.GetRow(`session`)
	if rNil != nil {
		t.Fatalf("should be `nil`")
	}
}

func TestBlock_MarshalSlice(t *testing.T) {
	block := NewBlock(`app`)
	block.AddRows(
		NewRow(`session`, `.example.com`),
	)

	lines := block.MarshalSlice()

	if len(lines) != 1 {
		t.Fatalf("should be `1`")
	}

	if lines[0] != `APP_SESSION=".example.com"` {
		t.Fatalf("should be `APP_SESSION=\".example.com\"`")
	}
}

func TestBlock_MarshalSlice2(t *testing.T) {
	block := NewBlock(`app`)
	block.
		SetComment(`Block for an Application Settings`).
		AddRows(
			NewRow(`session`, `.example.com`),
			NewRow(`URL`, `https://example.com`).SetComment(`Application URL`),
		)

	lines := block.MarshalSlice()

	if len(lines) != 4 {
		t.Fatalf("should be `%d`", len(lines))
	}

	if lines[0] != `###   ---[ Block for an Application Settings ]---   ###` {
		t.Fatalf("Wrong!")
	}

	if lines[1] != `APP_SESSION=".example.com"` {
		t.Fatalf("should be `APP_SESSION=\".example.com\"`")
	}

	if lines[2] != "# Application URL" {
		t.Fatalf("should be `# Application URL")
	}
	if lines[3] != "APP_URL=\"https://example.com\"" {
		t.Fatalf("should be `APP_URL=\"https://example.com\"")
	}

	SetCommentTemplate(`# <- `, ` ->`)

	lines = block.MarshalSlice()

	if lines[0] != `# <- Block for an Application Settings ->` {
		t.Fatal("Wrong!")
	}
}

func TestBlock_MergeBlock(t *testing.T) {

	block := NewBlock(`APP`)
	block.AddRows(NewRow(`session`, `test`), NewRow(`test`, `1`))

	block2 := NewBlock(``)
	block2.AddRows(NewRow(`session`, `new-test`), NewRow(`test`, `100`), NewRow(`test2`, `200`))

	block.MergeBlock(*block2)

	if block.Count() != 3 {
		t.Fatal(`Should be 3`)
	}

	if r := block.GetRow(`session`); r == nil || r.Value != `new-test` {
		t.Fatal(`Should be "new-test"`)
	}
	if r := block.GetRow(`test`); r == nil || r.Value != `100` {
		t.Fatal(`Should be "100"`)
	}
	if r := block.GetRow(`test2`); r == nil || r.Value != `200` {
		t.Fatal(`Should be "200"`)
	}
}

func TestBlock_MergeRow(t *testing.T) {

	block := NewBlock(`APP`)
	block.AddRows(NewRow(`session`, `test`), NewRow(`test`, `1`))

	block.MergeRow(*NewRow(`session`, `new-test`))
	block.MergeRow(*NewRow(`hash`, `new-hash`))

	if block.Count() != 3 {
		t.Fatal(`Should be 3`)
	}

	if r := block.GetRow(`session`); r == nil || r.Value != `new-test` {
		t.Fatal(`Should be "new-test"`)
	}

	if r := block.GetRow(`hash`); r == nil || r.Value != `new-hash` {
		t.Fatal(`Should be "new-hash"`)
	}
}

func TestBlock_AddFromBlock(t *testing.T) {

	block := NewBlock(`APP`)
	block.AddRows(NewRow(`session`, `test`), NewRow(`test`, `1`))

	block2 := NewBlock(``)
	block2.AddRows(NewRow(`session`, `new-test`), NewRow(`test`, `100`), NewRow(`test2`, `200`))

	block.AddFromBlock(*block2)

	if block.Count() != 3 {
		t.Fatal(`Should be 3`)
	}

	if r := block.GetRow(`session`); r == nil || r.Value != `test` {
		t.Fatal(`Should be "test"`)
	}

	if r := block.GetRow(`test`); r == nil || r.Value != `1` {
		t.Fatal(`Should be "1"`)
	}

	if r := block.GetRow(`test2`); r == nil || r.Value != `200` {
		t.Fatal(`Should be "200"`)
	}
}
