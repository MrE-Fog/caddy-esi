// Copyright 2015-present, Cyrill @ Schumacher.fm and the CoreStore contributors
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy of
// the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations under
// the License.

package esitesting

import (
	"io"
	"io/ioutil"
	"os"
)

// Tempfile returns a temporary file path.
func Tempfile(t interface {
	Fatal(args ...interface{})
}) (fileName string, clean func()) {
	f, err := ioutil.TempFile("", "caddyesi-")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.Name()); err != nil {
		t.Fatal(err)
	}

	return f.Name(), func() {
		if err := os.Remove(f.Name()); err != nil {
			t.Fatal(err)
		}
	}
}

// WriteXMLTempFile writes the body into the file and returns the file name and
// a clean up function. The filename always has the suffix ".xml".
func WriteXMLTempFile(t interface {
	Fatal(args ...interface{})
}, body io.WriterTo) (fileName string, clean func()) {
	f, err := ioutil.TempFile("", "caddyesi-")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := body.WriteTo(f); err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	newName := f.Name() + ".xml"
	if err := os.Rename(f.Name(), newName); err != nil {
		t.Fatal(err)
	}

	return newName, func() {
		if err := os.Remove(newName); err != nil {
			t.Fatal(err)
		}
	}
}
