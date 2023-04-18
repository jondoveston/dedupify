# Dedupify

Dedupify is a stupidly simple two pass file deduplicator. The file comparison
is done using the xxHash altorithm https://github.com/cespare/xxhash.

## Usage

The first argument is either `hash` or `search`.

The hash command will recursive walk from the current directory hashing every
file and output the file information as JSON on STDOUT (some human readable
messages and errors are output to STDERR). To update only files that have
changed the previous JSON can be piped in to prevent hashing the same files
again (a change in file size or mod time will trigger a rehash).

The optional second argument sets a minimum size filter for the files using
https://github.com/c2h5oh/datasize.

```
$ dedupify hash 1mb 1>dedupify1.json
Min size 1048576
Too small: Makefile
Too small: README.md
File Name: dedupify
File Size: 2544336
File ModTime: 1681843488
File Hash: 1683d4023d13c889
File HashTime: 1681843503

Too small: dedupify1.json
Too small: go.mod
Too small: go.sum
Too small: main.go

$ cat dedupify1.json
[
  {
    "path": "/home/user/dedupify/dedupify",
    "size": 2544336,
    "hash": "1683d4023d13c889",
    "mod_time": 1681843488,
    "hash_time": 1681843503
  }
]

$ cat dedupify1.json | dedupify hash 1mb 1>dedupify2.json
Min size 1048576
Too small: Makefile
Too small: README.md
File Name: dedupify
File Size: 2544336
Already hashed
File ModTime: 1681843488
File Hash: 1683d4023d13c889
File HashTime: 1681843503

Too small: dedupify1.json
Too small: dedupify2.json
Too small: go.mod
Too small: go.sum
Too small: main.go

$ touch dedupify

$ cat dedupify1.json | dedupify hash 1mb 1>dedupify2.json
Min size 1048576
Too small: Makefile
Too small: README.md
File Name: dedupify
File Size: 2544336
File ModTime: 1681843526
File Hash: 1683d4023d13c889
File HashTime: 1681843535

Too small: dedupify1.json
Too small: dedupify2.json
Too small: go.mod
Too small: go.sum
Too small: main.go
```

The search command will use the JSON piped in to search for files with the same
hash and display the results. Human readable messages are output to STDERR and a
list of the duplicate files to delete are output to STDOUT.

```
$ cp dedupify dedupify_copy

$ dedupify hash 1mb 1>dedupify.json
Min size 1048576
Too small: Makefile
Too small: README.md
File Name: dedupify
File Size: 2544336
File ModTime: 1681843526
File Hash: 1683d4023d13c889
File HashTime: 1681844016

File Name: dedupify_copy
File Size: 2544336
File ModTime: 1681844002
File Hash: 1683d4023d13c889
File HashTime: 1681844016

Too small: go.mod
Too small: go.sum
Too small: main.go

$ cat dedupify.json | dedupify search 1mb 1>/dev/null
Min size 1048576
Searching 2 files...
* /home/user/dedupify/dedupify
/home/user/dedupify/dedupify_copy

$ cat dedupify1.json | dedupify search 1mb 2>/dev/null
/home/user/dedupify/dedupify_copy
```

## Improvements

* Better code. Use cobra framework. Split into files. Write some tests etc.
* Add concurrency. The search code should benefit from concurrency and
  maybe the hashing code too.
* Add better keep file selection logic.
