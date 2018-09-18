package uploadserver

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/pierrre/imageserver"
	imageserver_source "github.com/pierrre/imageserver/source"
)

var (
	// Dir is the path to the directory containing the test data.
	Dir = initDir()

	// Images contains all images by filename.
	Images        = make(map[string]*imageserver.Image)
	MapTypeFormat = initMapTypeFormat()

	// Server is an Image Server that uses filename as source.
	Server = imageserver.Server(imageserver.ServerFunc(func(params imageserver.Params) (*imageserver.Image, error) {
		sess, err := session.NewSession(&aws.Config{Region: aws.String("us-east-1")})
		if err != nil {
			fmt.Println("Error creating session ", err)
			return nil, err
		}

		cre, err := sess.Config.Credentials.Get()
		if err != nil {
			fmt.Println("Error getting cred ", err)
			return nil, err
		}
		fmt.Println("Access key id: ", cre.AccessKeyID)
		source, err := params.GetString(imageserver_source.Param)
		if err != nil {
			return nil, err
		}

		file, err := os.OpenFile(Dir+"/"+source, os.O_RDONLY|os.O_CREATE, 0666)
		if err != nil {
			fmt.Println("Unable to open file ", err)
			return nil, err
		}

		defer file.Close()

		s3dl := s3manager.NewDownloader(sess)
		_, err = s3dl.Download(file, &s3.GetObjectInput{
			Bucket: aws.String("tripzozo-bucket"),
			Key:    aws.String(source),
		})

		// im, err := Get(source)
		if err != nil {
			return nil, &imageserver.ParamError{Param: imageserver_source.Param, Message: err.Error()}
		}

		im, err := loadImageFromName(source)
		if err != nil {
			return nil, &imageserver.ParamError{Param: imageserver_source.Param, Message: err.Error()}
		}
		return im, nil
	}))
)

// Get returns an Image for a name.
func Get(name string) (*imageserver.Image, error) {
	im, ok := Images[name]
	if !ok {
		im, err := loadImageFromName(name)
		if err != nil {
			return nil, fmt.Errorf("unknown image \"%s\"", name)
		}
		return im, nil
	}
	return im, nil
}

func loadImageFromName(name string) (*imageserver.Image, error) {
	filePath := filepath.Join(Dir, name)
	fmt.Printf(filePath)
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	filetype := http.DetectContentType(data)
	im := &imageserver.Image{
		Format: MapTypeFormat[filetype],
		Data:   data,
	}
	os.Remove(Dir + name)
	// Images[name] = im
	return im, nil
}

func initMapTypeFormat() map[string]string {
	typeFormat := make(map[string]string)
	typeFormat["image/jpeg"] = "jpeg"
	typeFormat["image/jpg"] = "jpg"
	typeFormat["image/png"] = "png"
	typeFormat["image/gif"] = "gif"
	return typeFormat
}

func initDir() string {
	_, currentFile, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(currentFile), "/tmp")
	fmt.Printf(path)
	return path
}

func loadImage(filename string, format string) *imageserver.Image {
	filePath := filepath.Join(Dir, filename)
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		panic(err)
	}
	im := &imageserver.Image{
		Format: format,
		Data:   data,
	}
	Images[filename] = im
	return im
}
