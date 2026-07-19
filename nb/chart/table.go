package chart

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/scttfrdmn/go-notebook/nb"
)

// Table renders a slice of structs (or a slice of maps, or a [][]string) as a
// clean HTML table — the WCAG-clean twin every chart should have, and often the
// right answer on its own for a handful of rows. Build it with [Rows] /
// [RowsWith]. It is the one form that emits text/html rather than SVG.
//
// Column headers come from the struct's exported field names, title-cased, unless
// a field carries a `chart:"Label"` tag (use `chart:"-"` to omit a field).
// Numeric columns are detected from the field type and right-aligned with tabular
// figures so digits line up; everything else is left-aligned.
type Table struct {
	data any
	opts Opts
}

// Rows builds a table from data, which must be a slice of structs, a slice of
// maps (keys become columns, unioned in first-seen order), or a [][]string (the
// first row is the header). Anything else renders as a one-cell notice.
func Rows(data any) Table { return Table{data: data} }

// RowsWith is [Rows] with options (only Title is used).
func RowsWith(opts Opts, data any) Table { return Table{data: data, opts: opts} }

// Equal lets the engine's change-detection compare two tables without a raw ==.
// Table holds `data any`; a struct with an interface field is *statically*
// comparable, so the engine would otherwise reach for == and panic at runtime
// when the interface holds a slice or map (the common case here). Providing
// Equal takes the engine's Equal-method branch instead, and reflect.DeepEqual
// handles whatever shape `data` actually is. (The other forms hold slice fields
// directly, so they're statically non-comparable and never hit this path.)
func (t Table) Equal(other any) bool {
	o, ok := other.(Table)
	if !ok {
		return false
	}
	return reflect.DeepEqual(t, o)
}

// Render draws the table as HTML.
func (t Table) Render() nb.Rendered {
	cols, rows, aligns := t.extract()

	var b strings.Builder
	b.WriteString(`<div style="font-family:system-ui,-apple-system,'Segoe UI',sans-serif;color:var(--tbl-ink,#0b0b0b)">`)
	// Scoped theme vars so the table reads on light and dark like the charts do.
	b.WriteString(`<style>` +
		`.nbtbl{border-collapse:collapse;width:100%;font-size:13px}` +
		`.nbtbl caption{text-align:left;font-weight:600;font-size:14px;padding:0 0 8px;color:var(--tbl-ink,#0b0b0b)}` +
		`.nbtbl th{text-align:left;font-weight:600;color:var(--tbl-muted,#52514e);border-bottom:1.5px solid var(--tbl-rule,#c3c2b7);padding:6px 12px 6px 0;white-space:nowrap}` +
		`.nbtbl td{padding:6px 12px 6px 0;border-bottom:1px solid var(--tbl-grid,#e1e0d9)}` +
		`.nbtbl td.num,.nbtbl th.num{text-align:right;font-variant-numeric:tabular-nums;padding-left:24px}` +
		`.nbtbl tr:last-child td{border-bottom:none}` +
		`@media (prefers-color-scheme:dark){` +
		`.nbtbl{--tbl-ink:#fff;--tbl-muted:#c3c2b7;--tbl-rule:#383835;--tbl-grid:#2c2c2a}}` +
		`</style>`)
	b.WriteString(`<table class="nbtbl">`)
	if t.opts.Title != "" {
		b.WriteString(`<caption>` + esc(t.opts.Title) + `</caption>`)
	}

	// Header.
	b.WriteString(`<thead><tr>`)
	for i, c := range cols {
		cls := ""
		if aligns[i] {
			cls = ` class="num"`
		}
		b.WriteString(`<th` + cls + `>` + esc(c) + `</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)

	// Body.
	for _, row := range rows {
		b.WriteString(`<tr>`)
		for i, cell := range row {
			cls := ""
			if i < len(aligns) && aligns[i] {
				cls = ` class="num"`
			}
			b.WriteString(`<td` + cls + `>` + esc(cell) + `</td>`)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table></div>`)
	return nb.HTML(b.String())
}

// extract turns the input into (column headers, string cells, per-column
// numeric-alignment flags), reflecting over whichever supported shape it is.
func (t Table) extract() (cols []string, rows [][]string, numeric []bool) {
	v := reflect.ValueOf(t.data)
	if v.Kind() != reflect.Slice || v.Len() == 0 {
		// Try [][]string / unknown: fall through to a friendly empty table.
		if ss, ok := t.data.([][]string); ok {
			return fromStrings(ss)
		}
		return []string{"(no rows)"}, nil, []bool{false}
	}

	switch elem := v.Index(0); elem.Kind() {
	case reflect.Struct:
		return t.fromStructs(v)
	case reflect.Map:
		return fromMaps(v)
	default:
		if ss, ok := t.data.([][]string); ok {
			return fromStrings(ss)
		}
		// Slice of scalars → a single "value" column.
		cols = []string{"value"}
		numeric = []bool{isNumericKind(elem.Kind())}
		for i := 0; i < v.Len(); i++ {
			rows = append(rows, []string{fmtCell(v.Index(i))})
		}
		return cols, rows, numeric
	}
}

// fromStructs reflects a []struct into columns from the field names/tags.
func (t Table) fromStructs(v reflect.Value) (cols []string, rows [][]string, numeric []bool) {
	et := v.Index(0).Type()
	var fields []int // exported, non-omitted field indices
	for i := 0; i < et.NumField(); i++ {
		f := et.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		tag := f.Tag.Get("chart")
		if tag == "-" {
			continue
		}
		name := tag
		if name == "" {
			name = titleCase(f.Name)
		}
		fields = append(fields, i)
		cols = append(cols, name)
		numeric = append(numeric, isNumericKind(f.Type.Kind()))
	}
	for r := 0; r < v.Len(); r++ {
		row := make([]string, len(fields))
		rv := v.Index(r)
		for j, fi := range fields {
			row[j] = fmtCell(rv.Field(fi))
		}
		rows = append(rows, row)
	}
	return cols, rows, numeric
}

// fromMaps reflects a []map into columns unioned across rows in first-seen order.
func fromMaps(v reflect.Value) (cols []string, rows [][]string, numeric []bool) {
	seen := map[string]int{}
	numByCol := map[string]bool{}
	for r := 0; r < v.Len(); r++ {
		iter := v.Index(r).MapRange()
		for iter.Next() {
			k := fmtCell(iter.Key())
			if _, ok := seen[k]; !ok {
				seen[k] = len(cols)
				cols = append(cols, k)
				numByCol[k] = true // assume numeric until a non-numeric value appears
			}
			val := iter.Value()
			if val.Kind() == reflect.Interface {
				val = val.Elem()
			}
			if val.IsValid() && !isNumericKind(val.Kind()) {
				numByCol[k] = false
			}
		}
	}
	numeric = make([]bool, len(cols))
	for c, i := range seen {
		numeric[i] = numByCol[c]
	}
	for r := 0; r < v.Len(); r++ {
		row := make([]string, len(cols))
		iter := v.Index(r).MapRange()
		for iter.Next() {
			row[seen[fmtCell(iter.Key())]] = fmtCell(iter.Value())
		}
		rows = append(rows, row)
	}
	return cols, rows, numeric
}

// fromStrings treats the first row as the header; columns are numeric if every
// body cell parses as a float.
func fromStrings(ss [][]string) (cols []string, rows [][]string, numeric []bool) {
	if len(ss) == 0 {
		return []string{"(no rows)"}, nil, []bool{false}
	}
	cols = ss[0]
	rows = ss[1:]
	numeric = make([]bool, len(cols))
	for i := range numeric {
		numeric[i] = true
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(numeric) && numeric[i] {
				if _, err := strconv.ParseFloat(strings.TrimSpace(cell), 64); err != nil && cell != "" {
					numeric[i] = false
				}
			}
		}
	}
	return cols, rows, numeric
}

// fmtCell renders a reflected value as a table cell string. Floats use the same
// clean formatter as the axes; other kinds fall back to their default string.
func fmtCell(v reflect.Value) string {
	if v.Kind() == reflect.Interface || v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return fmtNum(v.Float())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return group(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return group(strconv.FormatUint(v.Uint(), 10))
	case reflect.String:
		return v.String()
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	default:
		if v.IsValid() {
			return strings.TrimSpace(reflectString(v))
		}
		return ""
	}
}

// reflectString is a last-resort stringer that avoids fmt (kept off the cell-body
// WASM path); it handles the common Stringer case and otherwise the kind name.
func reflectString(v reflect.Value) string {
	if s, ok := v.Interface().(interface{ String() string }); ok {
		return s.String()
	}
	return v.Kind().String()
}

func isNumericKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// titleCase turns a Go field name into a header: "UnitPrice" → "Unit price",
// "ID" stays "ID". A light touch — splits camelCase on the lower→upper boundary
// and lowercases interior words, leaving all-caps runs (acronyms) intact.
func titleCase(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if i > 0 && isUpper(r) && !isUpper(runes[i-1]) {
			b.WriteByte(' ')
			b.WriteRune(toLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func toLower(r rune) rune {
	if isUpper(r) {
		return r + 32
	}
	return r
}
