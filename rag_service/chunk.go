package rag

import "strings"

// chunkText делит текст на фрагменты длиной примерно size рун с перекрытием overlap.
// Деление идёт по абзацам, длинные абзацы режутся по словам.
func chunkText(text string, size, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if size <= 0 {
		size = 1200
	}
	if overlap < 0 || overlap >= size {
		overlap = size / 8
	}

	paras := splitParagraphs(text)
	var chunks []string
	var cur []rune

	flush := func() {
		if len(cur) == 0 {
			return
		}
		chunks = append(chunks, strings.TrimSpace(string(cur)))
		if overlap > 0 && len(cur) > overlap {
			cur = append([]rune{}, cur[len(cur)-overlap:]...)
		} else {
			cur = cur[:0]
		}
	}

	for _, p := range paras {
		pr := []rune(p)
		// Абзац длиннее окна — режем по словам.
		if len(pr) > size {
			for _, w := range splitWords(p, size) {
				if len(cur)+len(w) > size {
					flush()
				}
				cur = append(cur, []rune(w+" ")...)
			}
			continue
		}
		if len(cur)+len(pr) > size {
			flush()
		}
		cur = append(cur, pr...)
		cur = append(cur, '\n')
	}
	if s := strings.TrimSpace(string(cur)); s != "" {
		chunks = append(chunks, s)
	}
	return chunks
}

func splitParagraphs(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []string
	for _, l := range raw {
		if s := strings.TrimSpace(l); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// splitWords режет длинный абзац на куски не длиннее size рун по границам слов.
func splitWords(p string, size int) []string {
	words := strings.Fields(p)
	var out []string
	var b strings.Builder
	for _, w := range words {
		if b.Len() > 0 && len([]rune(b.String()))+len([]rune(w)) > size {
			out = append(out, b.String())
			b.Reset()
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(w)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
