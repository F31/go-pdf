package doc

import (
	"bytes"
	"encoding/xml"
	"strconv"
	"strings"
)

type xmpFields struct {
	Title, Author, Subject, Keywords, Creator, Producer, CreateDate, ModifyDate string
}

func applyXMP(info *Info, data []byte) {
	if len(data) == 0 {
		return
	}
	f := parseXMP(data)
	if f.Title != "" {
		info.Title = f.Title
	}
	if f.Author != "" {
		info.Author = f.Author
	}
	if f.Subject != "" {
		info.Subject = f.Subject
	}
	if f.Keywords != "" {
		info.Keywords = f.Keywords
	}
	if f.Creator != "" {
		info.Creator = f.Creator
	}
	if f.Producer != "" {
		info.Producer = f.Producer
	}
	if f.CreateDate != "" {
		info.CreationDate = f.CreateDate
	}
	if f.ModifyDate != "" {
		info.ModDate = f.ModifyDate
	}
}

func parseXMP(data []byte) xmpFields {
	f, complete := parseXMPXML(data)
	if complete || f != (xmpFields{}) {
		return f
	}
	return parseXMPLight(data)
}

// parseXMPXML reads the common XMP fields through a real XML tokenizer. It
// keeps the last complete value for scalar fields and joins repeated RDF list
// items for creator/subject-like fields.
func parseXMPXML(data []byte) (xmpFields, bool) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []string
	values := map[string][]string{}
	for {
		tok, err := dec.Token()
		if err != nil {
			return finishXMP(values), err == nil
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			key := xmpKey(stack)
			if key == "" {
				continue
			}
			if text := strings.TrimSpace(string(t)); text != "" {
				values[key] = append(values[key], text)
			}
		}
	}
}

func xmpKey(stack []string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		switch strings.ToLower(stack[i]) {
		case "title":
			return "title"
		case "creator":
			return "creator"
		case "description":
			return "description"
		case "subject":
			return "subject"
		case "keywords":
			return "keywords"
		case "creatortool":
			return "creatortool"
		case "producer":
			return "producer"
		case "createdate":
			return "createdate"
		case "modifydate":
			return "modifydate"
		}
	}
	return ""
}

func finishXMP(values map[string][]string) xmpFields {
	first := func(key string) string {
		if vs := values[key]; len(vs) > 0 {
			return vs[0]
		}
		return ""
	}
	join := func(key, sep string) string {
		return strings.Join(values[key], sep)
	}
	keywords := join("keywords", ", ")
	if keywords == "" {
		keywords = join("subject", ", ")
	}
	return xmpFields{
		Title:      first("title"),
		Author:     join("creator", "; "),
		Subject:    join("description", "; "),
		Keywords:   keywords,
		Creator:    first("creatortool"),
		Producer:   first("producer"),
		CreateDate: first("createdate"),
		ModifyDate: first("modifydate"),
	}
}

func parseXMPLight(data []byte) xmpFields {
	return xmpFields{
		Title:      xmpField(data, "title"),
		Author:     xmpField(data, "creator"),
		Subject:    xmpField(data, "description"),
		Keywords:   firstNonEmpty(xmpField(data, "Keywords"), xmpField(data, "subject")),
		Creator:    xmpField(data, "CreatorTool"),
		Producer:   xmpField(data, "Producer"),
		CreateDate: xmpField(data, "CreateDate"),
		ModifyDate: xmpField(data, "ModifyDate"),
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func xmpField(data []byte, local string) string {
	open := findOpenTag(data, local, 0)
	if open < 0 {
		return ""
	}
	start := bytes.IndexByte(data[open:], '>')
	if start < 0 {
		return ""
	}
	start += open + 1
	close := findCloseTag(data, local, start)
	if close < 0 {
		return ""
	}
	return strings.TrimSpace(stripXMLMarkup(data[start:close]))
}

func findOpenTag(data []byte, local string, from int) int {
	for i := from; i < len(data); i++ {
		if data[i] != '<' || i+1 >= len(data) {
			continue
		}
		if data[i+1] == '/' || data[i+1] == '!' || data[i+1] == '?' {
			continue
		}
		j := i + 1
		for j < len(data) && data[j] != '>' && !isSpaceByte(data[j]) && data[j] != '/' {
			j++
		}
		name := data[i+1 : j]
		if localName(name) == local {
			return i
		}
	}
	return -1
}

func findCloseTag(data []byte, local string, from int) int {
	for i := from; i+2 < len(data); i++ {
		if data[i] != '<' || data[i+1] != '/' {
			continue
		}
		j := i + 2
		for j < len(data) && data[j] != '>' && !isSpaceByte(data[j]) {
			j++
		}
		if localName(data[i+2:j]) == local {
			return i
		}
	}
	return -1
}

func localName(name []byte) string {
	if k := bytes.LastIndexByte(name, ':'); k >= 0 {
		name = name[k+1:]
	}
	return string(bytes.TrimSpace(name))
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func stripXMLMarkup(data []byte) string {
	var out bytes.Buffer
	for i := 0; i < len(data); {
		if data[i] != '<' {
			out.WriteByte(data[i])
			i++
			continue
		}
		if bytes.HasPrefix(data[i:], []byte("<![CDATA[")) {
			end := bytes.Index(data[i+9:], []byte("]]>"))
			if end < 0 {
				out.Write(data[i+9:])
				break
			}
			out.Write(data[i+9 : i+9+end])
			i += 9 + end + 3
			continue
		}
		end := bytes.IndexByte(data[i:], '>')
		if end < 0 {
			break
		}
		i += end + 1
	}
	return unescapeXML(out.String())
}

func unescapeXML(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			out.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i:], ';')
		if end < 0 {
			out.WriteByte(s[i])
			i++
			continue
		}
		ent := s[i+1 : i+end]
		i += end + 1
		switch ent {
		case "amp":
			out.WriteByte('&')
		case "lt":
			out.WriteByte('<')
		case "gt":
			out.WriteByte('>')
		case "quot":
			out.WriteByte('"')
		case "apos":
			out.WriteByte('\'')
		default:
			if strings.HasPrefix(ent, "#x") || strings.HasPrefix(ent, "#X") {
				if n, err := strconv.ParseUint(ent[2:], 16, 32); err == nil {
					out.WriteRune(rune(n))
					continue
				}
			} else if strings.HasPrefix(ent, "#") {
				if n, err := strconv.ParseUint(ent[1:], 10, 32); err == nil {
					out.WriteRune(rune(n))
					continue
				}
			}
			out.WriteByte('&')
			out.WriteString(ent)
			out.WriteByte(';')
		}
	}
	return out.String()
}
