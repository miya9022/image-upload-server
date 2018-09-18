package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/disintegration/gift"
	"github.com/pierrre/githubhook"
	"github.com/pierrre/imageserver"
	imageserver_cache "github.com/pierrre/imageserver/cache"
	imageserver_cache_memory "github.com/pierrre/imageserver/cache/memory"
	imageserver_http "github.com/pierrre/imageserver/http"
	imageserver_http_crop "github.com/pierrre/imageserver/http/crop"
	imageserver_http_gamma "github.com/pierrre/imageserver/http/gamma"
	imageserver_http_gift "github.com/pierrre/imageserver/http/gift"
	imageserver_http_image "github.com/pierrre/imageserver/http/image"
	imageserver_image "github.com/pierrre/imageserver/image"
	_ "github.com/pierrre/imageserver/image/bmp"
	imageserver_image_crop "github.com/pierrre/imageserver/image/crop"
	imageserver_image_gamma "github.com/pierrre/imageserver/image/gamma"
	imageserver_image_gif "github.com/pierrre/imageserver/image/gif"
	imageserver_image_gift "github.com/pierrre/imageserver/image/gift"
	_ "github.com/pierrre/imageserver/image/jpeg"
	_ "github.com/pierrre/imageserver/image/png"
	_ "github.com/pierrre/imageserver/image/tiff"
	// imageserver_testdata "github.com/pierrre/imageserver/testdata"
	imageserver_http_cors "github.com/miya9022/image-upload-server/http"
	imageserver_upload "github.com/miya9022/image-upload-server/uploadserver"
)

var (
	flagHTTP                = ":8100"
	flagGitHubWebhookSecret string
	flagCache               = int64(128 * (1 << 20))
	flagMaxUploadSize       = int64(5 * (1 << 20))
	flagUploadPath          = "/uploadserver/tmp"
	server                  = imageserver_upload.Server
)

func main() {
	parseFlags()
	startHTTPServer()
}

func parseFlags() {
	flag.StringVar(&flagHTTP, "http", flagHTTP, "HTTP")
	flag.StringVar(&flagGitHubWebhookSecret, "github-webhook-secret", flagGitHubWebhookSecret, "GitHub webhook secret")
	flag.Int64Var(&flagCache, "cache", flagCache, "Cache")
	flag.Int64Var(&flagMaxUploadSize, "maxUploadSize", flagMaxUploadSize, "MaxUploadSize")
	flag.StringVar(&flagUploadPath, "uploadPath", flagUploadPath, "UploadPath")
	flag.Parse()
}

func startHTTPServer() {
	err := http.ListenAndServe(flagHTTP, newHTTPHandler())
	if err != nil {
		panic(err)
	}
}

func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.StripPrefix("/", newImageHTTPHandler()))
	mux.Handle("/favicon.ico", http.NotFoundHandler())
	mux.HandleFunc("/upload", uploadFileHandler())
	if h := newGitHubWebhookHTTPHandler(); h != nil {
		mux.Handle("/github_webhook", h)
	}
	return mux
}

func uploadFileHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			renderError(w, "INVALID REQUEST", http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, flagMaxUploadSize)
		if err := r.ParseMultipartForm(flagMaxUploadSize); err != nil {
			renderError(w, "FILE TOO BIG", http.StatusBadRequest)
			return
		}

		fileType := r.PostFormValue("type")
		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			renderError(w, "INVALID FILE", http.StatusBadRequest)
			return
		}

		defer file.Close()
		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			renderError(w, "INVALID FILE", http.StatusBadRequest)
			return
		}

		filetype := http.DetectContentType(fileBytes)
		if filetype != "image/jpeg" && filetype != "image/jpg" &&
			filetype != "image/gif" && filetype != "image/png" {
			renderError(w, "INVALID FILE TYPE", http.StatusBadRequest)
			return
		}

		var imgSrc image.Image
		if filetype == "image/png" {
			imgSrc, _ = png.Decode(bytes.NewReader(fileBytes))
		} else if filetype == "image/gif" {
			imgSrc, _ = gif.Decode(bytes.NewReader(fileBytes))
		} else {
			imgSrc, _ = jpeg.Decode(bytes.NewReader(fileBytes))
		}

		g := gift.New(
			gift.Resize(0, 800, gift.LanczosResampling),
			gift.UnsharpMask(1, 1, 0),
		)
		newImage := image.NewRGBA(g.Bounds(imgSrc.Bounds()))
		g.DrawAt(newImage, imgSrc, newImage.Bounds().Min, gift.OverOperator)

		// img := image.NewGray(image.Rect(0, 0, 100, 100))
		// img.Pix = fileBytes

		// img := &image.Gray{Pix: fileBytes, Stride: 0, Rect: image.Rect(0, 0, 0, 0)}

		// img := image.NewGray(image.Rect(0, 0, 100, 100))
		// copy(img.Pix, fileBytes)
		// img, _, _ := image.Decode(bytes.NewReader(fileBytes))
		// img, _ := jpeg.Decode(bytes.NewReader(fileBytes))

		// g := gift.New(
		// 	gift.Resize(0, 0, gift.LanczosResampling),
		// 	gift.UnsharpMask(1, 1, 0),
		// )
		// dst := image.NewRGBA(img.Bounds())
		// g.DrawAt(dst, img, dst.Bounds().Min, gift.CopyOperator)

		// buf := new(bytes.Buffer)
		// err1 := jpeg.Encode(buf, dst, nil)
		// if err1 != nil {
		// 	renderError(w, "CANT CONVERT IMAGE", http.StatusInternalServerError)
		// 	return
		// }
		// newFileBytes := buf.Bytes()

		fileName := randToken(12)
		fileEndings, err := mime.ExtensionsByType(filetype)
		if err != nil {
			renderError(w, "CANT READ FILE TYPE", http.StatusInternalServerError)
			return
		}
		fullFileName := fileName + fileEndings[0]
		_, currentFile, _, _ := runtime.Caller(0)
		path := filepath.Join(filepath.Dir(currentFile), flagUploadPath)
		newPath := filepath.Join(path, fullFileName)
		fmt.Printf("File Type: %s, File: %s\n", fileType, newPath)

		newFile, err := os.Create(newPath)
		if err != nil {
			renderError(w, "CANT WRITE FILE", http.StatusInternalServerError)
			return
		}

		defer newFile.Close()

		var opt jpeg.Options
		opt.Quality = 80
		if err := jpeg.Encode(newFile, newImage, &opt); err != nil {
			renderError(w, "CANT WRITE FILE", http.StatusInternalServerError)
			return
		}
		// w.Write([]byte(fullFileName))

		sess, err := session.NewSession(&aws.Config{
			Region: aws.String("us-east-1")},
		)
		if err != nil {
			renderError(w, "CANT CONNECT TO AWS SESSION", http.StatusInternalServerError)
			return
		}

		file, err = os.Open(newPath)
		if err != nil {
			renderError(w, "UNABLE TO OPEN FILE", http.StatusInternalServerError)
			return
		}

		defer file.Close()

		uploader := s3manager.NewUploader(sess)
		result, err := uploader.Upload(&s3manager.UploadInput{
			Bucket:      aws.String("tripzozo-bucket"),
			Key:         aws.String(fullFileName),
			Body:        file,
			ContentType: aws.String(filetype),
		})
		if err != nil {
			renderError(w, "CANT UPLOAD FILE", http.StatusInternalServerError)
			os.Remove(newPath)
			return
		}
		os.Remove(newPath)
		w.Write([]byte(fileName + "-" + result.Location))
	})
}

func renderError(w http.ResponseWriter, message string, statusCode int) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(message))
}

func randToken(len int) string {
	b := make([]byte, len)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func newGitHubWebhookHTTPHandler() http.Handler {
	if flagGitHubWebhookSecret == "" {
		return nil
	}
	return &githubhook.Handler{
		Secret: flagGitHubWebhookSecret,
		Delivery: func(event string, deliveryID string, payload interface{}) {
			if event == "push" {
				time.AfterFunc(5*time.Second, func() {
					os.Exit(0)
				})
			}
		},
	}
}

func newImageHTTPHandler() http.Handler {
	server = newServer()
	var handler http.Handler = &imageserver_http.Handler{
		Parser: imageserver_http.ListParser([]imageserver_http.Parser{
			&imageserver_http.SourcePathParser{},
			&imageserver_http_crop.Parser{},
			&imageserver_http_gift.RotateParser{},
			&imageserver_http_gift.ResizeParser{},
			&imageserver_http_image.FormatParser{},
			&imageserver_http_image.QualityParser{},
			&imageserver_http_gamma.CorrectionParser{},
		}),
		Server:   server,
		ETagFunc: imageserver_http.NewParamsHashETagFunc(sha256.New),
	}
	handler = &imageserver_http.ExpiresHandler{
		Handler: handler,
		Expires: 7 * 24 * time.Hour,
	}
	handler = &imageserver_http.CacheControlPublicHandler{
		Handler: handler,
	}
	handler = &imageserver_http_cors.CorsHandler{
		Handler: handler,
	}
	return handler
}

func newServer() imageserver.Server {
	srv := imageserver_upload.Server
	srv = newServerImage(srv)
	srv = newServerLimit(srv)
	srv = newServerCacheMemory(srv)
	return srv
}

func newServerImage(srv imageserver.Server) imageserver.Server {
	basicHdr := &imageserver_image.Handler{
		Processor: imageserver_image_gamma.NewCorrectionProcessor(
			imageserver_image.ListProcessor([]imageserver_image.Processor{
				&imageserver_image_crop.Processor{},
				&imageserver_image_gift.RotateProcessor{
					DefaultInterpolation: gift.CubicInterpolation,
				},
				&imageserver_image_gift.ResizeProcessor{
					DefaultResampling: gift.LanczosResampling,
					MaxWidth:          2048,
					MaxHeight:         2048,
				},
			}),
			true,
		),
	}
	gifHdr := &imageserver_image_gif.FallbackHandler{
		Handler: &imageserver_image_gif.Handler{
			Processor: &imageserver_image_gif.SimpleProcessor{
				Processor: imageserver_image.ListProcessor([]imageserver_image.Processor{
					&imageserver_image_crop.Processor{},
					&imageserver_image_gift.RotateProcessor{
						DefaultInterpolation: gift.NearestNeighborInterpolation,
					},
					&imageserver_image_gift.ResizeProcessor{
						DefaultResampling: gift.NearestNeighborResampling,
						MaxWidth:          1024,
						MaxHeight:         1024,
					},
				}),
			},
		},
		Fallback: basicHdr,
	}
	return &imageserver.HandlerServer{
		Server:  srv,
		Handler: gifHdr,
	}
}

func newServerLimit(srv imageserver.Server) imageserver.Server {
	return imageserver.NewLimitServer(srv, runtime.GOMAXPROCS(0)*2)
}

func newServerCacheMemory(srv imageserver.Server) imageserver.Server {
	if flagCache <= 0 {
		return srv
	}
	return &imageserver_cache.Server{
		Server:       srv,
		Cache:        imageserver_cache_memory.New(flagCache),
		KeyGenerator: imageserver_cache.NewParamsHashKeyGenerator(sha256.New),
	}
}
