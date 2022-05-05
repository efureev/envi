package envi

import (
	"strings"
)

var commentTemplateBeforeDefault = `###   ---[ `
var commentTemplateAfterDefault = ` ]---   ###`
var commentTemplateBefore = commentTemplateBeforeDefault
var commentTemplateAfter = commentTemplateAfterDefault

var blockIndent = 1

type Block struct {
	Prefix  string
	Comment string
	Rows    []*row
}

func (b Block) GetKey() string {
	return b.Prefix
}

func (b Block) Marshal() (str string, err error) {
	lines := b.MarshalSlice()
	if lines == nil {
		return
	}

	str = strings.Join(lines, "\n")
	str += strings.Repeat("\n", blockIndent)

	return
}

func (b Block) MarshalSlice() (lines []string) {
	if len(b.Rows) == 0 {
		return
	}

	if b.Comment != `` {
		comment := commentTemplateBefore + b.Comment + commentTemplateAfter
		lines = append(lines, comment)
	}
	for _, row := range b.Rows {
		rLines := row.MarshalSlice()

		if rLines != nil {
			lines = append(lines, rLines...)
		}
	}

	return
}

func (b *Block) SetComment(str string) *Block {
	b.Comment = str
	return b
}

func (b *Block) SetPrefix(str string) *Block {
	b.Prefix = normalizeKey(str)
	return b
}

func (b *Block) AddRows(rows ...*row) *Block {
	for _, r := range rows {
		b.addRow(r)
	}

	return b
}

func (b *Block) addRow(row *row) *Block {
	if !b.HasRow(row.Key) {
		row.block = b

		if b.Comment == `` && row.blockComment != `` {
			b.SetComment(row.blockComment)
			row.blockComment = ``
		}

		b.Rows = append(b.Rows, row)
	}

	return b
}

func (b *Block) addPrefixedRow(r *row) {
	if !strings.HasPrefix(r.Key, b.Prefix) {
		return
	}

	key := strings.Replace(r.Key, b.Prefix+`_`, ``, 1)
	r.Key = key
	b.AddRows(r)
}

func (b *Block) AddPrefixedRows(rows ...*row) *Block {
	for _, r := range rows {
		b.addPrefixedRow(r)
	}

	return b
}

func (b *Block) AddRow(key, value string) *Block {
	if !b.HasRow(key) {
		b.AddRows(NewRow(key, value))
	}
	return b
}

func (b Block) getRowByKey(key string) *row {
	for _, r := range b.Rows {
		if r.Key == key {
			return r
		}
	}

	return nil
}

func (b Block) GetRow(key string) *row {
	return b.getRowByKey(normalizeKey(key))
}

func (b Block) GetPrefixedRow(key string) *row {
	key = normalizeKey(key)
	if strings.HasPrefix(key, b.Prefix) {
		key = strings.Replace(key, b.Prefix+`_`, ``, 1)
	}

	return b.getRowByKey(key)
}

func (b *Block) MergeBlock(newBlock Block) {
	for _, r := range newBlock.Rows {
		existRow := b.GetRow(r.Key)
		if existRow == nil {
			b.AddRows(r)
		} else {
			existRow.Merge(*r)
		}
	}

	if b.Comment == `` && newBlock.Comment != `` {
		b.Comment = newBlock.Comment
	}
}
func (b *Block) MergeRow(mergeRow row) {
	existRow := b.GetRow(mergeRow.Key)
	if existRow != nil {
		existRow.Merge(mergeRow)
		return
	}
	b.addRow(&mergeRow)
}

func (b Block) HasRow(key string) bool {
	return b.GetRow(key) != nil
}

func (b *Block) AddFromBlock(newBlock Block) {
	for _, r := range newBlock.Rows {
		if !b.HasRow(r.Key) {
			b.AddRows(r)
		}
	}
}

func (b *Block) RemoveRowByIndex(ind int) {
	b.Rows = append(b.Rows[:ind], b.Rows[ind+1:]...)
}

func (b *Block) RemovePrefixedRow(key string) {
	key = normalizeKey(key)
	if strings.HasPrefix(key, b.Prefix) {
		key = strings.Replace(key, b.Prefix+`_`, ``, 1)
	}

	b.removeRow(key)
}

func (b *Block) removeRow(key string) {
	for i, r := range b.Rows {
		if r.Key == key {
			b.RemoveRowByIndex(i)
			return
		}
	}
}

func (b *Block) RemoveRow(key string) {
	b.removeRow(normalizeKey(key))
}

func (b Block) Count() int {
	return len(b.Rows)
}

func NewBlock(prefix string) *Block {
	return &Block{
		Prefix: normalizeKey(prefix),
	}
}

func SetCommentTemplate(before, after string) {
	commentTemplateBefore = before
	commentTemplateAfter = after
}

func SetCommentTemplateByDefault() {
	commentTemplateBefore = commentTemplateBeforeDefault
	commentTemplateAfter = commentTemplateAfterDefault
}

func SetIndent(val int) {
	blockIndent = val
}

func rowCommentMatchBlock(line string) (string, bool) {
	line = strings.TrimSpace(line)
	commentTemplateBeforeNew := strings.TrimSpace(commentTemplateBefore)
	commentTemplateAfterNew := strings.TrimSpace(commentTemplateAfter)

	if strings.HasPrefix(line, commentTemplateBeforeNew) && strings.HasSuffix(line, commentTemplateAfterNew) {
		line = strings.TrimPrefix(line, commentTemplateBeforeNew)
		line = strings.TrimSuffix(line, commentTemplateAfterNew)

		return strings.TrimSpace(line), true
	}

	return line, false
}
