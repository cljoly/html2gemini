# html2gemini

## A Go library to converts HTML into Gemini text/gemini (gemtext)

This is forked from https://jaytaylor.com/html2text with the following changes:

* output text/gemini format
* use footnote style references

## Introduction

Turns HTML into text/gemini to be served over gemini, or incorporated into a client.

html2gemini is a simple golang package for rendering HTML into plaintext.


## Download the package

```bash
go get github.com/LukeEmmet/html2gemini
```

## Example usage

See https://github.com/LukeEmmet/html2gmi-cli which is a practical command line application that uses this library.

To simplify the html passed to this library, you could simplify or sanitise it first, for example using https://github.com/philipjkim/goreadability

## Unit-tests

Running the unit-tests is straightforward and standard:

```bash
go test
```


# License

Permissive MIT license.


## Contact

Email: luke [at] marmaladefoo [dot] com

If you appreciate this library please feel free to drop me a line and tell me, and please send a note of appreciation to Jay Taylor (url below) who wrote the original html2text on which this is based, and who should receive most of the credit.

https://jaytaylor.com/html2text

