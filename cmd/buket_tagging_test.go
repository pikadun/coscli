package cmd

import (
	"coscli/util"
	"fmt"
	"testing"

	. "github.com/agiledragon/gomonkey/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/tencentyun/cos-go-sdk-v5"
)

func TestBucket_taggingCmd(t *testing.T) {
	fmt.Println("TestBucket_taggingCmd")
	testBucket = randStr(8)
	testAlias = testBucket + "-alias"
	setUp(testBucket, testAlias, testEndpoint)
	defer tearDown(testBucket, testAlias, testEndpoint)
	cmd := rootCmd
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	Convey("test coscli bucket_tagging", t, func() {
		Convey("success", func() {
			Convey("put", func() {
				args := []string{"bucket-tagging", "--method", "put",
					fmt.Sprintf("cos://%s", testAlias), "testkey#testval"}
				cmd.SetArgs(args)
				e := cmd.Execute()
				So(e, ShouldBeNil)
			})
			Convey("get", func() {
				args := []string{"bucket-tagging", "--method", "get",
					fmt.Sprintf("cos://%s", testAlias)}
				cmd.SetArgs(args)
				e := cmd.Execute()
				So(e, ShouldBeNil)
			})
			Convey("delete", func() {
				args := []string{"bucket-tagging", "--method", "delete",
					fmt.Sprintf("cos://%s", testAlias)}
				cmd.SetArgs(args)
				e := cmd.Execute()
				So(e, ShouldBeNil)
			})
		})
		Convey("fail", func() {
			Convey("put", func() {
				Convey("not enough arguments", func() {
					args := []string{"bucket-tagging", "--method", "put",
						fmt.Sprintf("cos://%s", testAlias)}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
				Convey("clinet err", func() {
					patches := ApplyFunc(util.NewClient, func(config *util.Config, param *util.Param, bucketName string) (client *cos.Client, err error) {
						return nil, fmt.Errorf("test put client error")
					})
					defer patches.Reset()
					args := []string{"bucket-tagging", "--method", "put",
						fmt.Sprintf("cos://%s", testAlias), "testval"}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
				Convey("invalid tag", func() {
					args := []string{"bucket-tagging", "--method", "put",
						fmt.Sprintf("cos://%s", testAlias), "testval"}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
				Convey("PutTagging failed", func() {
					args := []string{"bucket-tagging", "--method", "put",
						fmt.Sprintf("cos://%s", testAlias), "qcs:1#testval"}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
			})
			Convey("get", func() {
				Convey("not enough arguments", func() {
					args := []string{"bucket-tagging", "--method", "get"}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
				Convey("clinet err", func() {
					patches := ApplyFunc(util.NewClient, func(config *util.Config, param *util.Param, bucketName string) (client *cos.Client, err error) {
						return nil, fmt.Errorf("test get client error")
					})
					defer patches.Reset()
					args := []string{"bucket-tagging", "--method", "get",
						fmt.Sprintf("cos://%s", testAlias)}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
				Convey("get not exist", func() {
					args := []string{"bucket-tagging", "--method", "get",
						fmt.Sprintf("cos://%s", testAlias)}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
			})
			Convey("delete", func() {
				Convey("not enough arguments", func() {
					args := []string{"bucket-tagging", "--method", "delete"}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
				Convey("clinet err", func() {
					patches := ApplyFunc(util.NewClient, func(config *util.Config, param *util.Param, bucketName string) (client *cos.Client, err error) {
						return nil, fmt.Errorf("test delete client error")
					})
					defer patches.Reset()
					args := []string{"bucket-tagging", "--method", "delete",
						fmt.Sprintf("cos://%s", testAlias)}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
				Convey("delete bucket not exist", func() {
					args := []string{"bucket-tagging", "--method", "delete",
						fmt.Sprintf("cos://%s", "testAlias")}
					cmd.SetArgs(args)
					e := cmd.Execute()
					fmt.Printf(" : %s", e.Error())
					So(e, ShouldBeError)
				})
			})
		})
	})
}
