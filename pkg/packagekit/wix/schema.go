package wix

import (
	"encoding/xml"

	"github.com/pkg/errors"
)

type File struct {
	Id      string    `xml:",attr,omitempty"`
	Source  string    `xml:",attr,omitempty"`
	KeyPath YesNoType `xml:",attr,omitempty"`
}

type Component struct {
	Id    string `xml:",attr,omitempty"`
	Guid  string `xml:",attr,omitempty"`
	Files []File `xml:"File,omitempty"`
}

func (c *Component) RetFiles() []File {
	return c.Files
}

type ComponentRef struct {
	Id string `xml:",attr,omitempty"`
}

type ComponentGroup struct {
	Id            string         `xml:",attr,omitempty"`
	ComponentRefs []ComponentRef `xml:"ComponentRef,omitempty"`
}

type Directory struct {
	Id          string      `xml:",attr,omitempty"`
	Name        string      `xml:",attr,omitempty"`
	Components  []Component `xml:"Component,omitempty"`
	Directories []Directory `xml:"Directory,omitempty"`
}

func (d *Directory) RetFiles() []File {
	files := []File{}

	for _, c := range d.Components {
		files = append(files, c.RetFiles()...)
	}
	for _, d := range d.Directories {
		files = append(files, d.RetFiles()...)
	}
	return files

}

type DirectoryRef struct {
	Id          string      `xml:",attr,omitempty"`
	Name        string      `xml:",attr,omitempty"`
	Directories []Directory `xml:"Directory,omitempty"`
}

func (dr *DirectoryRef) RetFiles() []File {
	files := []File{}

	for _, d := range dr.Directories {
		files = append(files, d.RetFiles()...)
	}
	return files
}

type Fragment struct {
	DirectoryRefs   []DirectoryRef   `xml:"DirectoryRef,omitempty"`
	ComponentGroups []ComponentGroup `xml:"ComponentGroup,omitempty"`
}

func (f *Fragment) RetFiles() []File {
	files := []File{}

	for _, dr := range f.DirectoryRefs {
		files = append(files, dr.RetFiles()...)
	}

	return files
}

type Wix struct {
	XMLName   xml.Name   `xml:"http://schemas.microsoft.com/wix/2006/wi Wix"`
	Fragments []Fragment `xml:"Fragment"`
}

func (s *Wix) RetFiles() []File {
	// There is probably a more generic reflect way to do
	// this. But, this is a simple, if brute force method.
	files := []File{}

	for _, f := range s.Fragments {
		files = append(files, f.RetFiles()...)
	}
	return files

}

func FIXMEParseAppFiles(contents []byte) (*Wix, error) {
	v := &Wix{}

	if len(contents) == 0 {
		return nil, errors.New("No content")
	}

	err := xml.Unmarshal([]byte(contents), v)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling")
	}

	return v, nil
}
