package tent

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"net/textproto"
	"strconv"
	"strings"
)

type ReadLenSeeker interface {
	io.ReadSeeker
	Len() int64
}

type MultipartPostWriter struct {
	m *multipart.Writer
	i int
}

func NewMultipartPostWriter(w io.Writer) *MultipartPostWriter {
	return &MultipartPostWriter{m: multipart.NewWriter(w)}
}

func (w *MultipartPostWriter) WritePost(post *Post) error {
	data, _ := json.Marshal(post)
	part, err := w.m.CreatePart(mimeFileHeader("post", "post.json", post.contentType(), int64(len(data))))
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}

func (w *MultipartPostWriter) WriteAttachment(att *PostAttachment) error {
	part, err := w.m.CreatePart(mimeFileHeader(att.Category+"["+strconv.Itoa(w.i)+"]", att.Name, att.ContentType, att.Data.Len()))
	_, err = io.Copy(part, att.Data)
	return err
}

func (w *MultipartPostWriter) Close() error        { return w.m.Close() }
func (w *MultipartPostWriter) ContentType() string { return w.m.FormDataContentType() }

func mimeFileHeader(name, filename, contentType string, size int64) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+escapeQuotes(name)+`"; filename="`+escapeQuotes(filename)+`"`)
	h.Set("Content-Type", contentType)
	h.Set("Content-Length", strconv.FormatInt(size, 10))
	return h
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}
