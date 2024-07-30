package dto

import (
	"bytes"
	"strconv"
)

type CertInfo struct {
	ID     int64
	Name   string
	Region string
	Domain string
}

func (c *CertInfo) String() string {
	buf := bytes.NewBufferString("{")
	buf.WriteString("ID:")
	buf.WriteString(strconv.FormatInt(c.ID, 10))
	buf.WriteString(",")

	buf.WriteString("Name:")
	buf.WriteString(c.Name)
	buf.WriteString(",")

	buf.WriteString("Region:")
	buf.WriteString(c.Region)
	buf.WriteString(",")

	buf.WriteString("Domain:")
	buf.WriteString(c.Domain)
	buf.WriteString(",")

	buf.WriteString("}")
	return buf.String()
}
