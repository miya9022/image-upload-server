package uploadserver

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"net/http"
	"runtime"

	"github.com/pierrre/imageserver"
	imageserver_source "github.com/pierrre/imageserver/source"
)

var (
	// Dir is the path to the directory containing the test data.
	Dir = initDir()

	// Images contains all images by filename.
	Images = make(map[string]*imageserver.Image)
	MapTypeFormat = initMapTypeFormat()

	// Server is an Image Server that uses filename as source.
	Server = imageserver.Server(imageserver.ServerFunc(func(params imageserver.Params) (*imageserver.Image, error) {
		source, err := params.GetString(imageserver_source.Param)
		if err != nil {
			return nil, err
		}
		im, err := Get(source)
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
	Images[name] = im
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
