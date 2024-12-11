package util

import (
	"context"
	"fmt"
	"github.com/olekukonko/tablewriter"
	"github.com/tencentyun/cos-go-sdk-v5"
	"net/url"
	"os"
	"strconv"
	"time"
)

func ListParts(c *cos.Client, cosUrl StorageUrl, limit int, uploadId string) error {
	var err error
	var parts []cos.Object

	total := 0
	isTruncated := true
	var partNumberMarker string

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"PartNumber", "ETag", "Last Modified", "Size"})
	table.SetBorder(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)

	for isTruncated && total < limit {
		table.ClearRows()
		queryLimit := 1000
		if limit-total < 1000 {
			queryLimit = limit - total
		}

		err, parts, isTruncated, partNumberMarker = GetPartsListForLs(c, cosUrl, uploadId, partNumberMarker, queryLimit)

		if err != nil {
			return fmt.Errorf("list uploads error : %v", err)
		}

		for _, part := range parts {
			utcTime, err := time.Parse(time.RFC3339, part.LastModified)
			if err != nil {
				return fmt.Errorf("Error parsing time:%v", err)
			}
			table.Append([]string{strconv.Itoa(part.PartNumber), part.ETag, utcTime.Local().Format(time.RFC3339), formatBytes(float64(part.Size))})
			total++
		}

		if !isTruncated || total >= limit {
			table.SetFooter([]string{"", "", "", fmt.Sprintf("Total: %d", total)})
			table.Render()
			break
		}
		table.Render()

		// 重置表格
		table = tablewriter.NewWriter(os.Stdout)
		table.SetBorder(false)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetAutoWrapText(false)
	}

	return nil
}

func ListUploads(c *cos.Client, cosUrl StorageUrl, limit int, filters []FilterOptionType) error {
	var err error
	var uploads []struct {
		Key          string
		UploadID     string `xml:"UploadId"`
		StorageClass string
		Initiator    *cos.Initiator
		Owner        *cos.Owner
		Initiated    string
	}

	total := 0
	isTruncated := true
	var keyMarker, uploadIDMarker string

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Key", "Upload ID", "Type", "Initiate time"})
	table.SetBorder(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)

	for isTruncated && total < limit {
		table.ClearRows()
		queryLimit := 1000
		if limit-total < 1000 {
			queryLimit = limit - total
		}

		err, uploads, isTruncated, uploadIDMarker, keyMarker = GetUploadsListForLs(c, cosUrl, uploadIDMarker, keyMarker, queryLimit, true)

		if err != nil {
			return fmt.Errorf("list uploads error : %v", err)
		}

		for _, upload := range uploads {
			upload.Key, _ = url.QueryUnescape(upload.Key)
			if cosObjectMatchPatterns(upload.Key, filters) {
				table.Append([]string{upload.Key, upload.UploadID, upload.StorageClass, upload.Initiated})
				total++
			}
		}

		if !isTruncated || total >= limit {
			table.SetFooter([]string{"", "", "", fmt.Sprintf("Total: %d", total)})
			table.Render()
			break
		}
		table.Render()

		// 重置表格
		table = tablewriter.NewWriter(os.Stdout)
		table.SetBorder(false)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetAutoWrapText(false)
	}

	return nil
}

func GetUploadsListForLs(c *cos.Client, cosUrl StorageUrl, uploadIDMarker, keyMarker string, limit int, recursive bool) (err error, uploads []struct {
	Key          string
	UploadID     string `xml:"UploadId"`
	StorageClass string
	Initiator    *cos.Initiator
	Owner        *cos.Owner
	Initiated    string
}, isTruncated bool, nextUploadIDMarker, nextKeyMarker string) {
	prefix := cosUrl.(*CosUrl).Object
	delimiter := ""
	if !recursive {
		delimiter = "/"
	}

	opt := &cos.ListMultipartUploadsOptions{
		Delimiter:      delimiter,
		EncodingType:   "url",
		Prefix:         prefix,
		MaxUploads:     limit,
		KeyMarker:      keyMarker,
		UploadIDMarker: uploadIDMarker,
	}
	res, err := tryGetUploads(c, opt)
	if err != nil {
		return
	}

	uploads = res.Uploads
	isTruncated = res.IsTruncated
	nextUploadIDMarker, _ = url.QueryUnescape(res.NextUploadIDMarker)
	nextKeyMarker, _ = url.QueryUnescape(res.NextKeyMarker)

	return
}

func GetPartsListForLs(c *cos.Client, cosUrl StorageUrl, uploadId, partNumberMarker string, limit int) (err error, parts []cos.Object, isTruncated bool, nextPartNumberMarker string) {
	name := cosUrl.(*CosUrl).Object

	opt := &cos.ObjectListPartsOptions{
		EncodingType:     "url",
		MaxParts:         strconv.Itoa(limit),
		PartNumberMarker: partNumberMarker,
	}

	res, err := tryGetParts(c, name, uploadId, opt)
	if err != nil {
		return
	}

	parts = res.Parts
	isTruncated = res.IsTruncated
	nextPartNumberMarker, _ = url.QueryUnescape(res.NextPartNumberMarker)

	return
}

func GetUploadsListRecursive(c *cos.Client, prefix string, limit int, include string, exclude string) (uploads []UploadInfo, err error) {
	opt := &cos.ListMultipartUploadsOptions{
		Delimiter:      "",
		EncodingType:   "",
		Prefix:         prefix,
		MaxUploads:     limit,
		KeyMarker:      "",
		UploadIDMarker: "",
	}

	isTruncated := true
	keyMarker := ""
	uploadIDMarker := ""
	for isTruncated {
		opt.KeyMarker = keyMarker
		opt.UploadIDMarker = uploadIDMarker

		res, _, err := c.Bucket.ListMultipartUploads(context.Background(), opt)
		if err != nil {
			return uploads, err
		}

		for _, u := range res.Uploads {
			uploads = append(uploads, UploadInfo{
				Key:       u.Key,
				UploadID:  u.UploadID,
				Initiated: u.Initiated,
			})
		}

		if limit > 0 {
			isTruncated = false
		} else {
			isTruncated = res.IsTruncated
			keyMarker = res.NextKeyMarker
			uploadIDMarker = res.NextUploadIDMarker
		}
	}

	if len(include) > 0 {
		uploads = MatchUploadPattern(uploads, include, true)
	}
	if len(exclude) > 0 {
		uploads = MatchUploadPattern(uploads, exclude, false)
	}

	return uploads, nil
}
