// Copyright 2013 Matthew Endsley
// All rights reserved
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted providing that the following conditions
// are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE AUTHOR ``AS IS'' AND ANY EXPRESS OR
// IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED.  IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY
// DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS
// OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION)
// HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING
// IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

package rollinglog

import (
	"io"
	"os"
	"path"
	"time"
)

// Create a new io.WriteCloser that targets a rolling log file. Uses path as a
// template, adding the current date.
//		data/server.log becomes data/2006/01/2006-01-02/server.log
func New(tz *time.Location, filepath string, mode os.FileMode, dirmode os.FileMode) io.WriteCloser {
	chFile := make(chan *os.File)
	chErr := make(chan error)
	chClosed := make(chan struct{})

	go func() {
		defer close(chFile)
		for {
			select {
			case <-chClosed:
				return
			default:
			}

			now := time.Now().In(tz)
			p := path.Dir(filepath) + now.Format("/2006/01/2006-01-02/") + path.Base(filepath)
			if err := os.MkdirAll(path.Dir(p), dirmode); err != nil && !os.IsExist(err) {
				select {
				case chErr <- err:
				case <-chClosed:
				}
				return
			}

			f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, mode)
			if err != nil {
				select {
				case chErr <- err:
				case <-chClosed:
				}
				return
			}

			select {
			case chFile <- f:
			case <-chClosed:
			}

			// wait for tomorrow
			tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			time.Sleep(tomorrow.Sub(now))
		}
	}()

	return &rollingFile{
		f:        <-chFile,
		chFile:   chFile,
		chErr:    chErr,
		chClosed: chClosed,
	}
}

type rollingFile struct {
	f        *os.File
	lastErr  error
	chFile   <-chan *os.File
	chErr    <-chan error
	chClosed chan<- struct{}
}

func (rf *rollingFile) Write(p []byte) (int, error) {
	select {
	case rf.lastErr = <-rf.chErr:
	case f := <-rf.chFile:
		rf.f.Close()
		rf.f = f
	default:
	}

	if rf.lastErr != nil {
		return 0, rf.lastErr
	}

	return rf.f.Write(p)
}

func (rf *rollingFile) Close() error {
	if rf.f != nil {
		rf.f.Close()
	}
	rf.lastErr = io.EOF
	close(rf.chClosed)
	return nil
}