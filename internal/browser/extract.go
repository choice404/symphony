package browser

import (
	"fmt"
	"strings"
)

// width is where rendered lines wrap
const width = 100

// extractScript walks the visible document in order and returns text, links, and fields, tagging every field with a number the page can address it by
const extractScript = `(() => {
  const out = { url: location.href, title: document.title, items: [], links: [], inputs: [] };
  const block = new Set(['P','DIV','SECTION','ARTICLE','HEADER','FOOTER','NAV','MAIN','ASIDE','UL','OL','LI','TABLE','TR','H1','H2','H3','H4','H5','H6','BR','HR','PRE','BLOCKQUOTE','FORM','FIELDSET','DD','DT','DL','FIGURE','FIGCAPTION','DETAILS','SUMMARY','TD','TH','LABEL','OPTION']);
  const skip = new Set(['SCRIPT','STYLE','NOSCRIPT','TEMPLATE','svg','SVG','HEAD','IFRAME','CANVAS','VIDEO','AUDIO','OBJECT','PATH']);
  const hidden = (el) => {
    if (el.hidden) return true;
    const s = getComputedStyle(el);
    return s.display === 'none' || s.visibility === 'hidden';
  };
  let links = 0;
  let fields = 0;
  const push = (t, v) => out.items.push({ t: t, v: v === undefined ? '' : String(v) });
  const walk = (node) => {
    if (node.nodeType === 3) {
      const s = node.nodeValue.replace(/\s+/g, ' ');
      if (s.trim()) push('text', s);
      return;
    }
    if (node.nodeType !== 1) return;
    const tag = node.tagName;
    if (skip.has(tag) || hidden(node)) return;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') {
      if (node.type === 'hidden') return;
      fields++;
      node.setAttribute('data-symphony-field', String(fields));
      out.inputs.push({ n: fields, type: node.type || tag.toLowerCase(), name: node.name || node.id || '', placeholder: node.placeholder || node.getAttribute('aria-label') || '', value: node.value || '' });
      push('break');
      push('field', fields);
      push('break');
      return;
    }
    if (tag === 'BUTTON' || (node.getAttribute('role') === 'button' && tag !== 'A')) {
      fields++;
      node.setAttribute('data-symphony-field', String(fields));
      out.inputs.push({ n: fields, type: 'button', name: (node.innerText || node.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim(), placeholder: '', value: '' });
      push('field', fields);
      return;
    }
    if (tag === 'IMG') {
      if (node.alt) push('text', '[' + node.alt + ']');
      return;
    }
    const isBlock = block.has(tag);
    if (isBlock) push('break');
    if (/^H[1-6]$/.test(tag)) push('text', '#'.repeat(Number(tag[1])) + ' ');
    if (tag === 'A' && node.href && !node.href.startsWith('javascript:')) {
      links++;
      out.links.push({ n: links, text: (node.innerText || node.textContent || '').replace(/\s+/g, ' ').trim(), href: node.href });
      push('link', links);
    }
    for (const c of node.childNodes) walk(c);
    if (isBlock) push('break');
  };
  walk(document.body);
  return out;
})()`

// item is one piece of the walked document
type item struct {
	// text, link, field, or break
	T string `json:"t"`
	// The text, or the number of the link or field
	V string `json:"v"`
}

// Link is one link on the page
type Link struct {
	// The number the page shows
	N int `json:"n"`
	// The text
	Text string `json:"text"`
	// The absolute href
	Href string `json:"href"`
}

// Field is one input, textarea, select, or button on the page
type Field struct {
	// The number the page shows
	N int `json:"n"`
	// The type, text, password, checkbox, button, and so on
	Type string `json:"type"`
	// The name or id, or a button's label
	Name string `json:"name"`
	// The placeholder or aria label
	Placeholder string `json:"placeholder"`
	// The current value
	Value string `json:"value"`
}

// Doc is a page read as text
type Doc struct {
	// The url
	URL string `json:"url"`
	// The title
	Title string `json:"title"`
	// The pieces in order
	Items []item `json:"items"`
	// The links
	Links []Link `json:"links"`
	// The fields
	Inputs []Field `json:"inputs"`
}

/**
 * Extract
 * Reads the tab's document as text with numbered links and fields
 * @return Doc, error
 **/
func (t *Tab) Extract() (Doc, error) {
	var d Doc
	if err := t.Eval(extractScript, &d); err != nil {
		return Doc{}, fmt.Errorf("browser: read page: %w", err)
	}
	return d, nil
}

/**
 * FieldSelector
 * Returns the selector of a numbered field
 * @param n {int} - the number
 * @return string
 **/
func FieldSelector(n int) string {
	return fmt.Sprintf(`[data-symphony-field="%d"]`, n)
}

/**
 * Field
 * Finds a field by number
 * @param n {int} - the number
 * @return Field, bool
 **/
func (d Doc) Field(n int) (Field, bool) {
	for _, f := range d.Inputs {
		if f.N == n {
			return f, true
		}
	}
	return Field{}, false
}

/**
 * Link
 * Finds a link by number
 * @param n {int} - the number
 * @return Link, bool
 **/
func (d Doc) Link(n int) (Link, bool) {
	for _, l := range d.Links {
		if l.N == n {
			return l, true
		}
	}
	return Link{}, false
}

// Line is one rendered line and the first link or field on it
type Line struct {
	// The text
	Text string
	// link:n, field:n, or empty
	Key string
}

/**
 * Render
 * Turns the walked document into wrapped lines, links as [n] before their text and fields as bracketed descriptions
 * @return []Line
 **/
func (d Doc) Render() []Line {
	// The lines, built from a run of words and a key for the run
	lines := make([]Line, 0, 64)
	var run []string
	key := ""
	flush := func() {
		text := strings.TrimSpace(strings.Join(run, " "))
		run = nil
		k := key
		key = ""
		if text == "" {
			// One blank line between blocks, never two
			if len(lines) > 0 && lines[len(lines)-1].Text != "" {
				lines = append(lines, Line{})
			}
			return
		}
		for _, w := range wrap(text, width) {
			lines = append(lines, Line{Text: w, Key: k})
			k = ""
		}
	}
	for _, it := range d.Items {
		switch it.T {
		case "text":
			run = append(run, strings.TrimSpace(it.V))
		case "link":
			run = append(run, "["+it.V+"]")
			if key == "" {
				key = "link:" + it.V
			}
		case "field":
			var n int
			_, _ = fmt.Sscanf(it.V, "%d", &n)
			if f, ok := d.Field(n); ok {
				run = append(run, describe(f))
			}
			if key == "" {
				key = "field:" + it.V
			}
		case "break":
			flush()
		}
	}
	flush()
	// Trim a leading and a trailing blank
	for len(lines) > 0 && lines[0].Text == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1].Text == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

/**
 * describe
 * Spells a field for a line
 * @param f {Field} - the field
 * @return string
 **/
func describe(f Field) string {
	label := f.Name
	if f.Placeholder != "" {
		label = f.Placeholder
	}
	switch f.Type {
	case "button", "submit":
		return fmt.Sprintf("{%d %s}", f.N, label)
	case "checkbox", "radio":
		return fmt.Sprintf("{%d %s [%s]}", f.N, label, f.Type)
	case "password":
		return fmt.Sprintf("{%d %s: ****}", f.N, label)
	}
	value := f.Value
	if value == "" {
		value = "_"
	}
	return fmt.Sprintf("{%d %s: %s}", f.N, label, value)
}

/**
 * wrap
 * Breaks text into lines no wider than w on spaces
 * @param text {string} - the text
 * @param w {int} - the width
 * @return []string
 **/
func wrap(text string, w int) []string {
	words := strings.Fields(text)
	out := make([]string, 0, 4)
	cur := ""
	for _, word := range words {
		if cur == "" {
			cur = word
			continue
		}
		if len([]rune(cur))+1+len([]rune(word)) > w {
			out = append(out, cur)
			cur = word
			continue
		}
		cur += " " + word
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
