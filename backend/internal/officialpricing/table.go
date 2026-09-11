package officialpricing

import (
	"errors"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

func elements(node *html.Node, tag string) []*html.Node {
	var result []*html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			result = append(result, n)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return result
}

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "sup" || n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == html.TextNode {
			builder.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && (n.Data == "br" || n.Data == "p" || n.Data == "div") {
			builder.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

// Expand row/column spans to a bounded grid so header meanings remain attached
// to prices when the documentation changes the number of listed models.
func tableGrid(table *html.Node) ([][]string, error) {
	rows := elements(table, "tr")
	if len(rows) > 128 {
		return nil, errors.New("官方价格表行数过多")
	}
	grid := make([][]string, len(rows))
	occupied := make([][]bool, len(rows))
	for i := range grid {
		grid[i] = make([]string, 32)
		occupied[i] = make([]bool, 32)
	}
	width := 0
	for y, row := range rows {
		x := 0
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type != html.ElementNode || (cell.Data != "td" && cell.Data != "th") {
				continue
			}
			for x < 32 && occupied[y][x] {
				x++
			}
			cols, height := 1, 1
			for _, attr := range cell.Attr {
				if attr.Key != "colspan" && attr.Key != "rowspan" {
					continue
				}
				value, err := strconv.Atoi(attr.Val)
				if err != nil || value < 1 || value > 128 {
					return nil, errors.New("官方价格表跨度无效")
				}
				if attr.Key == "colspan" {
					cols = value
				} else {
					height = value
				}
			}
			if x+cols > 32 || y+height > len(rows) {
				return nil, errors.New("官方价格表超出边界")
			}
			text := nodeText(cell)
			for dy := 0; dy < height; dy++ {
				for dx := 0; dx < cols; dx++ {
					if occupied[y+dy][x+dx] {
						return nil, errors.New("官方价格表单元格重叠")
					}
					grid[y+dy][x+dx] = text
					occupied[y+dy][x+dx] = true
				}
			}
			x += cols
			if x > width {
				width = x
			}
		}
	}
	for i := range grid {
		grid[i] = grid[i][:width]
	}
	return grid, nil
}
