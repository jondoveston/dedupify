package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/cespare/xxhash/v2"
)

// var (
// 	version = "0.0.1"
// 	commit  = "HEAD"
// 	date    = "now"
// 	builtBy = "jondoveston"
// )

var size int64 = 0
var fs []File

type File struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Hash     string `json:"hash"`
	ModTime  int64  `json:"mod_time"`
	HashTime int64  `json:"hash_time"`
}

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			j, _ := json.MarshalIndent(fs, "", "  ")
			fmt.Println(string(j))
			os.Exit(0)
		}
	}()

	currentDirectory, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

  // require at least one argument
	if len(os.Args) < 2 {
		return
	}

  // parse stdin json
	var fsin []File
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		bytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		if len(bytes) > 0 {
			err = json.Unmarshal(bytes, &fsin)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

  // third argument is the optional minimum file size
	if len(os.Args) == 3 {
		var v datasize.ByteSize
		err := v.UnmarshalText([]byte(os.Args[2]))
		if err != nil {
			log.Fatal(err)
		}
		size = int64(v.Bytes())
		fmt.Fprintf(os.Stderr, "Min size %d\n", size)
	}

  // first argument is the command: hash or search
	if os.Args[1] == "hash" {
		hash(currentDirectory, fsin)
		j, _ := json.MarshalIndent(fs, "", "  ")
		fmt.Println(string(j))
	} else if os.Args[1] == "search" {
		search(fsin)
	}
}

// search for duplicate files
func search(fs []File) {
	fmt.Fprintf(os.Stderr, "Searching %d files...\n", len(fs))

  // groups files by hash
	lookup := make(map[string][]File)
	for _, f := range fs {
		if f.Size <= size {
			continue
		}
		fs, ok := lookup[f.Hash]
		if ok {
			fs = append(fs, f)
			lookup[f.Hash] = fs
		} else {
			lookup[f.Hash] = []File{f}
		}
	}

  // look for groups with more than one file
	for _, fs := range lookup {
		if len(fs) > 1 {
			keep := selectFile(fs)
			fmt.Fprintf(os.Stderr, "* %s\n", keep.Path)
			for _, f := range fs {
				if f.Path != keep.Path {
					fmt.Fprintf(os.Stderr, "%s\n", f.Path)
					fmt.Fprintf(os.Stdout, "%s\n", f.Path)
				}
			}
			fmt.Fprintln(os.Stderr)
		}
	}
}

// recursively hash files
func hash(path string, fsin []File) {
  // build lookup table for files already hashed
	lookup := make(map[string]File)
	for _, f := range fsin {
		lookup[f.Path] = f
	}

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Fatalf(err.Error())
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() <= size {
			fmt.Fprintf(os.Stderr, "Too small: %s\n", info.Name())
			return nil
		}

		fmt.Fprintf(os.Stderr, "File Name: %s\n", info.Name())
		fmt.Fprintf(os.Stderr, "File Size: %d\n", info.Size())

		var hashString string
		var hashTime int64

		if f, ok := lookup[path]; ok {
      if f.HashTime > info.ModTime().Unix() && f.Size == info.Size() {
        fmt.Fprintln(os.Stderr, "Already hashed")
        hashString = f.Hash
        hashTime = f.HashTime
      }
		}

		if hashString == "" {
			f, err := os.Open(path)
			if err != nil {
				log.Fatal(err)
			}
			defer func() {
				_ = f.Close()
			}()

			buf := make([]byte, 1024*1024)
			hash := xxhash.New()

			for {
				n, err := f.Read(buf)
				if err != nil {
					if err != io.EOF {
						log.Fatal(err)
					}
					break
				}
				_, err = hash.Write(buf[:n])
				if err != nil {
					log.Fatal(err)
				}
			}
			hashTime = time.Now().Unix()
			hashString = fmt.Sprintf("%016x", hash.Sum64())
		}

		if hashTime == 0 {
			hashTime = time.Now().Unix()
		}

		fmt.Fprintf(os.Stderr, "File ModTime: %d\n", info.ModTime().Unix())
		fmt.Fprintf(os.Stderr, "File Hash: %s\n", hashString)
		fmt.Fprintf(os.Stderr, "File HashTime: %d\n", hashTime)

		fs = append(fs, File{
			Path:     path,
			Size:     info.Size(),
			ModTime:  info.ModTime().Unix(),
			Hash:     hashString,
			HashTime: hashTime,
		})
		fmt.Fprintln(os.Stderr)

		return nil
	})
	if err != nil {
		log.Fatalf(err.Error())
	}
}

// select the file to keep
func selectFile(fs []File) File {
	var keep []File

  // do not keep files with "copy" in the name
	for i, f := range fs {
		if strings.Contains(strings.ToLower(f.Path), " copy") {
			continue
		}
		keep = append(keep, fs[i])
	}

  // sort by ascending modification time
	if len(keep) > 1 {
		sort.Slice(keep, func(i, j int) bool {
			return keep[i].ModTime < keep[j].ModTime
		})
	}

  // return the oldest keep file
	if len(keep) > 0 {
		return keep[0]
	} else {
		return fs[0]
	}
}
