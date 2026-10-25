package reading

import (
	"context"
	"encoding/xml"
	"io"
	"strings"
)

// OpenClassicComic reads a CBR or CB7 through the bounded member reader and the
// same page model as CBZ. Names come from the archive's real image members and
// ComicInfo.xml when present; nothing is invented and the format is refused
// unless its pinned decoder is linked in.
func OpenClassicComic(ctx context.Context, format string, ra io.ReaderAt, size int64) (*Comic, error) {
	members, err := openMembers(ctx, format, ra, size)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(members.Members()))
	for _, member := range members.Members() {
		names = append(names, member.Name)
	}
	if len(names) == 0 {
		_ = members.Close()
		return nil, ErrInvalid
	}
	sortMembersStable(names)
	c := &Comic{members: members, Names: names}
	if body, ok := memberNamed(members, "ComicInfo.xml"); ok {
		var expanded int64
		raw, err := boundedMemberRead(ctx, body, &expanded)
		if err != nil {
			_ = members.Close()
			return nil, err
		}
		var info struct {
			XMLName                           xml.Name
			Title, Writer, Manga, LanguageISO string
			Format                            []struct {
				XMLName xml.Name
				Value   string `xml:",chardata"`
				Inner   string `xml:",innerxml"`
			} `xml:"Format"`
		}
		if err = decodeXML(raw, &info); err != nil {
			_ = members.Close()
			return nil, err
		}
		c.Metadata.Title, c.Metadata.Author, c.Metadata.Language = info.Title, info.Writer, info.LanguageISO
		if info.Manga == "YesAndRightToLeft" {
			c.Metadata.Direction = "rtl"
		}
		if len(c.Metadata.Title) > 4096 || len(c.Metadata.Author) > 4096 || len(c.Metadata.Language) > 64 {
			_ = members.Close()
			return nil, ErrLimit
		}
		c.Metadata.PublicationTypeField = "ComicInfo/Format"
		if info.XMLName.Space == "" && info.XMLName.Local == "ComicInfo" {
			values := []string{}
			scalar := true
			for _, field := range info.Format {
				if field.XMLName.Space == "" && field.XMLName.Local == "Format" {
					values = append(values, field.Value)
					scalar = scalar && !strings.Contains(field.Inner, "<")
				}
			}
			c.Metadata.PublicationTypeRaw, c.Metadata.PublicationTypeComplete = boundedTypeEvidence(values)
			c.Metadata.PublicationTypeComplete = c.Metadata.PublicationTypeComplete && scalar
			if c.Metadata.PublicationTypeComplete && len(values) == 1 {
				c.Metadata.ExplicitPublicationType = explicitPublicationType(values)
			}
		}
	}
	return c, nil
}

func memberNamed(a memberArchive, name string) (archiveMember, bool) {
	for _, member := range a.Members() {
		if member.Name == name {
			return member, true
		}
	}
	return archiveMember{}, false
}

func sortMembersStable(names []string) {
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && naturalLess(names[j], names[j-1]); j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
}
