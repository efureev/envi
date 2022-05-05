package envi

import (
	"bufio"
	"io"
	"os"
	"sort"
	"strings"
)

type EnvBlock interface {
	GetKey() string
	Marshal() (string, error)
	MarshalSlice() (lines []string)
}

var groupRowsGreaterThen = 0

type Env []EnvBlock

func (e *Env) addRowsForce(rows ...EnvBlock) {
	*e = append(*e, rows...)
}

func Unmarshal(str string) (Env, error) {
	return Parse(strings.NewReader(str))
}

func Parse(r io.Reader) (env Env, err error) {
	rows, err := ParseRows(r)
	if err != nil {
		return
	}
	env = SortByBlocks(rows)
	env.Sorting()

	return
}

func ParseRows(r io.Reader) (rows map[string]*row, err error) {
	var lines []string
	rows = make(map[string]*row)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err = scanner.Err(); err != nil {
		return
	}

	l := len(lines)
	preLines := ``
	blockComment := ``

	for i := 0; i < l; i++ {
		trimmedLine := strings.TrimSpace(lines[i])
		if len(trimmedLine) == 0 {
			continue
		}

		if strings.HasPrefix(trimmedLine, "#") {
			if blkCmt, ok := rowCommentMatchBlock(trimmedLine); ok {
				blockComment = blkCmt
			} else {
				commentedRow, cErr := isCommentedRow(trimmedLine)
				if cErr == nil {
					commentedRow.SetComment(lineToComment(preLines))
					preLines = ``
					if origRow, ok := rows[commentedRow.Key]; ok {
						origRow.AddShadow(commentedRow.Value)
					} else {
						rows[commentedRow.Key] = commentedRow
					}
				} else {
					if cErr == ErrWrongLineFormat {
						preLines += trimmedLine + "\n"
					} else if cErr == ErrEmptyString {
						continue
					} else {
						err = cErr
						return
					}
				}
			}

			continue
		}

		key, value, comment, rErr := parseLine(preLines + trimmedLine)

		preLines = ``
		if rErr != nil {
			err = rErr
			return
		}

		row := NewRow(key, value).SetComment(comment)
		row.blockComment = blockComment
		blockComment = ``

		if origRow, ok := rows[key]; ok {
			if origRow.commented {
				row.AddShadows(origRow.shadows)
				rows[key] = row
			} else {
				//origRow.Merge(*row)
				origRow.AddShadow(row.Value)
			}
		} else {
			rows[key] = row
		}
	}

	return
}

func SortByBlocks(rows map[string]*row) (env Env) {

	rowsToBlock := make(map[string]map[string]*row)

	for _, r := range rows {
		blockPrefix, rowName := suspectBlock(r.Key)
		if blockPrefix == `` {
			env.addRowsForce(r)
		} else {
			rowsMap, ok := rowsToBlock[blockPrefix]
			if !ok {
				rowsMap = make(map[string]*row)
			}

			rowsMap[rowName] = r
			rowsToBlock[blockPrefix] = rowsMap
		}
	}

	blocks := make(map[string]*Block)
	for blockPrefix, rowMap := range rowsToBlock {
		l := len(rowMap)
		i := 0
		for rowPostfix, r := range rowMap {
			if l <= groupRowsGreaterThen {
				env.addRowsForce(r)
				continue
			}

			block, ok := blocks[blockPrefix]
			if !ok {
				block = NewBlock(blockPrefix)
				blocks[blockPrefix] = block
				env.addRowsForce(block)
			}

			r.Key = rowPostfix
			block.AddRows(r)

			i++
			if i == l {
				sort.SliceStable(block.Rows, func(i, j int) bool {
					return block.Rows[i].Key < block.Rows[j].Key
				})
			}
		}
	}

	return
}

func suspectBlock(key string) (blockPrefix, rowName string) {
	chunks := strings.Split(key, `_`)
	if len(chunks) > 1 {
		return chunks[0], strings.Join(chunks[1:], `_`)
	}

	return ``, key
}

func (e Env) Sorting() {
	sort.SliceStable(e, func(i, j int) bool {
		return e[i].GetKey() < e[j].GetKey()
	})
}

func (e Env) Marshal() (string, error) {
	var res string
	for _, item := range e {
		str, err := item.Marshal()
		if err != nil {
			return res, err
		}
		if str != `` {
			res += "\n" + str
		}
	}

	return strings.TrimSpace(res), nil
}

func (e Env) MarshalToSlice() (res []string) {
	for _, item := range e {
		if lines := item.MarshalSlice(); lines != nil {
			res = append(res, item.MarshalSlice()...)
		}
	}

	return
}

func (e Env) Counts() (blocks, rows int) {
	for _, item := range e {
		switch item.(type) {
		case *Block:
			blocks++
		case *row:
			rows++
		}
	}
	return
}

func (e Env) Count() (total int) {
	for _, item := range e {
		switch eItem := item.(type) {
		case *Block:
			total += eItem.Count()
		case *row:
			total++
		}
	}

	return
}

func (e Env) BlocksCount() int {
	b, _ := e.Counts()
	return b
}

func (e Env) RowsCount() int {
	_, r := e.Counts()
	return r
}

func (e Env) GetBlock(prefix string) *Block {
	prefix = strings.ToUpper(prefix)
	for _, item := range e {
		switch block := item.(type) {
		case *Block:
			if block.Prefix == prefix {
				return block
			}
		default:
			continue
		}
	}

	return nil
}

func (e Env) Get(key string) *row {
	key = normalizeKey(key)
	blockPrefix, rowName := suspectBlock(key)
	if blockPrefix != `` {
		block := e.GetBlock(blockPrefix)
		if block != nil {
			return block.GetRow(rowName)
		}
	}

	for _, item := range e {
		switch r := item.(type) {
		case *row:
			if r.Key == key {
				return r
			}
		default:
			continue
		}
	}

	return nil
}

func (e Env) GetRowsByPrefix(prefix string) (list []*row) {
	for _, item := range e {
		switch r := item.(type) {
		case *row:
			if strings.HasPrefix(r.Key, prefix+`_`) {
				list = append(list, r)
			}
		}
	}

	return
}

func (e *Env) addBlock(blockToAdd *Block, merge bool) {
	block := e.GetBlock(blockToAdd.GetKey())
	if block == nil {
		*e = append(*e, blockToAdd)
		return
	}

	if merge {
		block.MergeBlock(*blockToAdd)
	} else {
		block.AddFromBlock(*blockToAdd)
	}
}

func (e *Env) addRow(r *row) {
	blockPrefix, nameRow := suspectBlock(r.Key)

	if blockPrefix == `` {
		e.addRowsForce(r)
		return
	}

	block := e.GetBlock(blockPrefix)
	if block != nil {
		block.addPrefixedRow(r)
		return
	}

	rowsForGroup := e.GetRowsByPrefix(blockPrefix)

	if len(rowsForGroup) == 0 {
		e.addRowsForce(r)
		return
	}

	for _, r := range rowsForGroup {
		e.removeRowFromRoot(r.Key)
	}

	block = NewBlock(blockPrefix)
	block.AddPrefixedRows(rowsForGroup...)
	r.Key = nameRow
	block.AddRows(r)

	e.addRowsForce(block)
}

func (e *Env) mergeRow(mergeRow *row) {
	r := e.Get(mergeRow.Key)
	if r != nil {
		r.Merge(*mergeRow)
		return
	}

	e.addRow(r)
}

func (e *Env) Add(rows ...EnvBlock) {
	for _, item := range rows {
		switch r := item.(type) {
		case *Block:
			e.addBlock(r, false)
		case *row:
			e.addRow(r)
		}
	}
}

func (e *Env) Merge(eMerge Env) {
	e.MergeItems(eMerge...)
}

func (e *Env) MergeItems(rows ...EnvBlock) {
	for _, item := range rows {
		switch r := item.(type) {
		case *Block:
			e.addBlock(r, true)
		case *row:
			e.mergeRow(r)
		}
	}
}

func (e *Env) RemoveItemByIndex(ind int) {
	*e = append((*e)[:ind], (*e)[ind+1:]...)
}

func (e *Env) removeRowFromRoot(key string) {
	for i, item := range *e {
		switch r := item.(type) {
		case *row:
			if r.Key == key {
				e.RemoveItemByIndex(i)

				return
			}
		}
	}
}

func (e *Env) RemoveBlock(key string) {
	key = normalizeKey(key)
	for i, item := range *e {
		switch r := item.(type) {
		case *Block:
			if r.Prefix == key {
				e.RemoveItemByIndex(i)

				return
			}
		}
	}
}

func (e *Env) RemoveRow(key string) {
	key = normalizeKey(key)

	blockPrefix, rowName := suspectBlock(key)

	if blockPrefix != `` {
		block := e.GetBlock(blockPrefix)
		block.removeRow(rowName)
		return
	}

	e.removeRowFromRoot(key)
}

func (e Env) ToSlice() (list []*row) {
	for _, item := range e {
		switch tItem := item.(type) {
		case *row:
			list = append(list, tItem)
		case *Block:
			list = append(list, tItem.Rows...)
		}
	}
	return
}

func (e Env) SetEnv(override bool) {
	for _, r := range e.ToSlice() {
		key := r.GetFullKey()
		val := r.Value

		if override {
			os.Setenv(key, val)
		} else {
			if _, present := os.LookupEnv(key); !present {
				os.Setenv(key, val)
			}
		}
	}

}

func (e Env) Save(filename string) (err error) {
	var f *os.File
	f, err = os.Create(filename)

	if err != nil {
		return
	}
	defer f.Close()

	var lines string
	lines, err = e.Marshal()

	if err != nil {
		return
	}
	_, err = f.WriteString(lines)
	if err != nil {
		return
	}

	return
}

/*func (e Env) String() (str string) {

	str, _ = e.Marshal()

	return
}
*/
func loadFromFile(filename string) (rows map[string]*row, err error) {
	var f *os.File
	f, err = os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	rows, err = ParseRows(f)
	if err != nil {
		return
	}

	return
}

func Load(filenames ...string) (Env, error) {
	if len(filenames) == 0 {
		filenames = []string{".env"}
	}
	rows := make(map[string]*row)
	for _, filename := range filenames {
		if rowsToMerge, err := loadFromFile(filename); err != nil {
			return nil, err
		} else {
			mergeRowMap(rows, rowsToMerge)
		}
	}

	env := SortByBlocks(rows)
	env.Sorting()

	return env, nil
}

func LoadFromMap(list map[string]string) Env {
	env := Env{}
	for key, value := range list {
		row := NewRow(key, value)
		env.addRow(row)
	}

	return env
}

func GroupRowsGreaterThen(val int) {
	groupRowsGreaterThen = val
}
