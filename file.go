package gotgbot

import (
	"encoding/json"
	"errors"
	"io"
)

// InputFile (https://core.telegram.org/bots/api#inputfile)
//
// This object represents the contents of a file to be uploaded.
// Must be posted using multipart/form-data in the usual way that files are uploaded via the browser.
type InputFile interface {
	Attach(name string, data map[string]FileReader) (string, error)
	justFiles()
}

// InputString (https://core.telegram.org/bots/api#inputfile)
//
// This object represents a publicly accessible URL to be reused, or a file_id already available on telegram servers.
type InputString interface {
	Attach(name string, data map[string]FileReader) (string, error)
	justStrings()
}

// InputFileOrString (https://core.telegram.org/bots/api#inputfile)
//
// This object represents the contents of a file to be uploaded, or a publicly accessible URL to be reused.
// Files must be posted using multipart/form-data in the usual way that files are uploaded via the browser.
type InputFileOrString interface {
	Attach(name string, data map[string]FileReader) (string, error)
}

var (
	_ InputFileOrString = FileString{}
	_ InputString       = FileString{}

	_ InputFileOrString = FileReader{}
	_ InputFile         = FileReader{}
)

type FileString struct {
	Value string
}

func (f FileString) justStrings() {}

func (f FileString) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.Value)
}

func (f FileString) Attach(_ string, _ map[string]FileReader) (string, error) {
	return f.Value, nil
}

type FileReader struct {
	Name string
	Data io.Reader
}

var ErrAttachmentKeyAlreadyExists = errors.New("key already exists")

func (f FileReader) Attach(key string, data map[string]FileReader) (string, error) {
	if _, ok := data[key]; ok {
		return "", ErrAttachmentKeyAlreadyExists
	}
	data[key] = f
	return "attach://" + key, nil
}

func (f FileReader) justFiles() {}

// InputFileURL is used to send a file on the internet via a publicly accessible HTTP URL.
func InputFileURL(url string) InputFileOrString {
	return FileString{Value: url}
}

// InputFileID is used to send a file that is already present on telegram's servers, using its telegram file_id.
func InputFileID(fileID string) InputFileOrString {
	return FileString{Value: fileID}
}

// InputFileReader is used to send a file by a reader interface; such as a filehandle from os.Open().
//
// For example:
//
//	f, err := os.Open("some_file.go")
//	if err != nil {
//		return fmt.Errorf("failed to open file: %w", err)
//	}
//
//	m, err := b.SendDocument(<chat_id>, gotgbot.InputFileReader("source.go", f), <opts>)
func InputFileReader(name string, r io.Reader) InputFile {
	return FileReader{Name: name, Data: r}
}
