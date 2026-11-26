package bundle

import (
	"fmt"
	"io"
	"strings"
)

// RenderText 以“轨道类型 x 语言”表格形式输出矩阵。
func RenderText(w io.Writer, m Matrix) {
	type row struct {
		kind  Kind
		cells map[string]*Cell
	}
	rows := make([]row, 0, len(AllKinds))
	for _, k := range AllKinds {
		rows = append(rows, row{k, m.Cells[k]})
	}

	widths := make([]int, len(m.Languages))
	for i, lang := range m.Languages {
		widths[i] = len(lang)
	}
	grid := make([][]string, len(rows))
	for r, item := range rows {
		grid[r] = make([]string, len(m.Languages))
		for c, lang := range m.Languages {
			grid[r][c] = cellLabel(item.cells[lang])
			if len(grid[r][c]) > widths[c] {
				widths[c] = len(grid[r][c])
			}
		}
	}

	header := pad("KIND\\LANG", 10)
	sep := strings.Repeat("-", 10)
	for c, lang := range m.Languages {
		header += " | " + pad(lang, widths[c])
		sep += "-+-" + strings.Repeat("-", widths[c])
	}
	fmt.Fprintln(w, header)
	fmt.Fprintln(w, sep)
	for r, item := range rows {
		line := pad(string(item.kind), 10)
		for c := range m.Languages {
			line += " | " + pad(grid[r][c], widths[c])
		}
		fmt.Fprintln(w, line)
	}
}

func cellLabel(c *Cell) string {
	if c == nil || !c.Present {
		return "-"
	}
	n := len(c.Tracks)
	if n == 1 {
		return "x"
	}
	return fmt.Sprintf("x%d", n)
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// RenderWarnings 将警告逐条写入 w。
func RenderWarnings(w io.Writer, warnings []string) {
	for _, msg := range warnings {
		fmt.Fprintf(w, "WARNING: %s\n", msg)
	}
}
