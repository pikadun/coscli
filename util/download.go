package util

import (
	"context"
	"fmt"
	logger "github.com/sirupsen/logrus"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type DownloadOptions struct {
	RateLimiting float32
	PartSize     int64
	ThreadNum    int
	SnapshotDb   *leveldb.DB
	SnapshotPath string
}

func Download(c *cos.Client, cosUrl StorageUrl, fileUrl StorageUrl, fo *FileOperations) {

	startT := time.Now().UnixNano() / 1000 / 1000

	fo.Monitor.init(fo.CpType)
	chProgressSignal = make(chan chProgressSignalType, 10)
	go progressBar(fo)

	if cosUrl.(*CosUrl).Object != "" && !strings.HasSuffix(cosUrl.(*CosUrl).Object, CosSeparator) {
		// 单对象下载
		index := strings.LastIndex(cosUrl.(*CosUrl).Object, "/")
		prefix := ""
		relativeKey := cosUrl.(*CosUrl).Object
		if index > 0 {
			prefix = cosUrl.(*CosUrl).Object[:index+1]
			relativeKey = cosUrl.(*CosUrl).Object[index+1:]
		}
		// 获取文件信息
		resp, err := getHead(c, cosUrl.(*CosUrl).Object)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				// 文件不在cos上
				logger.Fatalf("Object not found : %v", err)
			}
			logger.Fatalf("Head object err : %v", err)
		}

		fo.Monitor.updateScanSizeNum(resp.ContentLength, 1)
		fo.Monitor.setScanEnd()
		freshProgress()

		// 下载文件
		skip, err, isDir, size, msg := singleDownload(c, fo, objectInfoType{prefix, relativeKey, resp.ContentLength, resp.Header.Get("Last-Modified")}, cosUrl, fileUrl)
		fo.Monitor.updateMonitor(skip, err, isDir, size)
		if err != nil {
			logger.Fatalf("%s failed: %v", msg, err)
		}
	} else {
		// 多对象下载
		batchDownloadFiles(c, cosUrl, fileUrl, fo)
	}

	closeProgress()
	fmt.Printf(fo.Monitor.progressBar(true, normalExit))

	endT := time.Now().UnixNano() / 1000 / 1000
	PrintTransferStats(startT, endT, fo)
}

func batchDownloadFiles(c *cos.Client, cosUrl StorageUrl, fileUrl StorageUrl, fo *FileOperations) {
	chObjects := make(chan objectInfoType, ChannelSize)
	chError := make(chan error, fo.Operation.Routines)
	chListError := make(chan error, 1)

	// 扫描cos对象大小及数量
	go getCosObjectList(c, cosUrl, nil, nil, fo, true, false)
	// 获取cos对象列表
	go getCosObjectList(c, cosUrl, chObjects, chListError, fo, false, true)

	for i := 0; i < fo.Operation.Routines; i++ {
		go downloadFiles(c, cosUrl, fileUrl, fo, chObjects, chError)
	}

	completed := 0
	for completed <= fo.Operation.Routines {
		select {
		case err := <-chListError:
			if err != nil {
				if fo.Operation.FailOutput {
					writeError(err.Error(), fo)
				}
			}
			completed++
		case err := <-chError:
			if err == nil {
				completed++
			} else {
				if fo.Operation.FailOutput {
					writeError(err.Error(), fo)
				}
			}
		}
	}
}

func downloadFiles(c *cos.Client, cosUrl, fileUrl StorageUrl, fo *FileOperations, chObjects <-chan objectInfoType, chError chan<- error) {
	for object := range chObjects {
		var skip, isDir bool
		var err error
		var size int64
		var msg string
		for retry := 0; retry <= fo.Operation.ErrRetryNum; retry++ {
			skip, err, isDir, size, msg = singleDownload(c, fo, object, cosUrl, fileUrl)
			if err == nil {
				break // Download succeeded, break the loop
			} else {
				if retry < fo.Operation.ErrRetryNum {
					if fo.Operation.ErrRetryInterval == 0 {
						// If the retry interval is not specified, retry after a random interval of 1~10 seconds.
						time.Sleep(time.Duration(rand.Intn(10)+1) * time.Second)
					} else {
						time.Sleep(time.Duration(fo.Operation.ErrRetryInterval) * time.Second)
					}
				}
			}
		}

		fo.Monitor.updateMonitor(skip, err, isDir, size)
		if err != nil {
			chError <- fmt.Errorf("%s failed: %w", msg, err)
			continue
		}
	}

	chError <- nil
}

func singleDownload(c *cos.Client, fo *FileOperations, objectInfo objectInfoType, cosUrl, fileUrl StorageUrl) (skip bool, rErr error, isDir bool, size int64, msg string) {
	skip = false
	rErr = nil
	isDir = false
	size = objectInfo.size
	object := objectInfo.prefix + objectInfo.relativeKey

	localFilePath := DownloadPathFixed(objectInfo.relativeKey, fileUrl.ToString())
	msg = fmt.Sprintf("\nDownload %s to %s", getCosUrl(cosUrl.(*CosUrl).Bucket, object), localFilePath)

	_, err := os.Stat(localFilePath)

	// 是文件夹则直接创建并退出
	if size == 0 && strings.HasSuffix(object, "/") {
		rErr = os.MkdirAll(localFilePath, 0755)
		isDir = true
		return
	}

	if err == nil {
		// 文件存在再判断是否需要跳过
		// 仅sync命令执行skip
		if fo.Command == CommandSync {
			absLocalFilePath, _ := filepath.Abs(localFilePath)
			snapshotKey := getDownloadSnapshotKey(absLocalFilePath, cosUrl.(*CosUrl).Bucket, cosUrl.(*CosUrl).Object)
			skip, err = skipDownload(snapshotKey, c, fo, localFilePath, objectInfo.lastModified, object)
			if err != nil {
				rErr = err
			}

			if skip {
				return
			}

		}
	}

	// 不是文件夹则创建父目录
	err = createParentDirectory(localFilePath)
	if err != nil {
		rErr = err
		return
	}

	// 未跳过则通过监听更新size
	size = 0

	// 开始下载文件
	opt := &cos.MultiDownloadOptions{
		Opt: &cos.ObjectGetOptions{
			ResponseContentType:        "",
			ResponseContentLanguage:    "",
			ResponseExpires:            "",
			ResponseCacheControl:       "",
			ResponseContentDisposition: "",
			ResponseContentEncoding:    "",
			Range:                      "",
			IfModifiedSince:            "",
			XCosSSECustomerAglo:        "",
			XCosSSECustomerKey:         "",
			XCosSSECustomerKeyMD5:      "",
			XOptionHeader:              nil,
			XCosTrafficLimit:           (int)(fo.Operation.RateLimiting * 1024 * 1024 * 8),
			Listener:                   &CosListener{fo},
		},
		PartSize:       fo.Operation.PartSize,
		ThreadPoolSize: fo.Operation.ThreadNum,
		CheckPoint:     true,
		CheckPointFile: "",
	}

	resp, err := c.Object.Download(context.Background(), object, localFilePath, opt)
	if err != nil {
		rErr = err
		return
	}

	// 下载完成记录快照信息
	if fo.Operation.SnapshotPath != "" {
		lastModified := resp.Header.Get("Last-Modified")
		if lastModified == "" {
			return
		}

		// 解析时间字符串
		objectModifiedTime, err := time.Parse(time.RFC3339, lastModified)
		if err != nil {
			objectModifiedTime, err = time.Parse(time.RFC1123, lastModified)
			if err != nil {
				rErr = err
				return
			}

		}

		absLocalFilePath, _ := filepath.Abs(localFilePath)
		snapshotKey := getDownloadSnapshotKey(absLocalFilePath, cosUrl.(*CosUrl).Bucket, cosUrl.(*CosUrl).Object)
		fo.SnapshotDb.Put([]byte(snapshotKey), []byte(strconv.FormatInt(objectModifiedTime.Unix(), 10)), nil)
	}

	return
}
