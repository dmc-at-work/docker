package engine

import (
	"bytes"
	"io"
	"strings"
	"fmt"
	"encoding/json"
)

// A job is the fundamental unit of work in the docker engine.
// Everything docker can do should eventually be exposed as a job.
// For example: execute a process in a container, create a new container,
// download an archive from the internet, serve the http api, etc.
//
// The job API is designed after unix processes: a job has a name, arguments,
// environment variables, standard streams for input, output and error, and
// an exit status which can indicate success (0) or error (anything else).
//
// One slight variation is that jobs report their status as a string. The
// string "0" indicates success, and any other strings indicates an error.
// This allows for richer error reporting.
// 
type Job struct {
	Eng	*Engine
	Name	string
	Args	[]string
	env	[]string
	Stdin	io.ReadCloser
	Stdout	io.WriteCloser
	Stderr	io.WriteCloser
	handler	func(*Job) string
	status	string
}

// Run executes the job and blocks until the job completes.
// If the job returns a failure status, an error is returned
// which includes the status.
func (job *Job) Run() error {
	job.Logf("{")
	defer func() {
		job.Logf("}")
	}()
	if job.handler == nil {
		job.status = "command not found"
	} else {
		job.status = job.handler(job)
	}
	if job.status != "0" {
		return fmt.Errorf("%s: %s", job.Name, job.status)
	}
	return nil
}

// String returns a human-readable description of `job`
func (job *Job) String() string {
	s := fmt.Sprintf("%s.%s(%s)", job.Eng, job.Name, strings.Join(job.Args, ", "))
	// FIXME: if a job returns the empty string, it will be printed
	// as not having returned.
	// (this only affects String which is a convenience function).
	if job.status != "" {
		var okerr string
		if job.status == "0" {
			okerr = "OK"
		} else {
			okerr = "ERR"
		}
		s = fmt.Sprintf("%s = %s (%s)", s, okerr, job.status)
	}
	return s
}

func (job *Job) Getenv(key string) (value string) {
        for _, kv := range job.env {
                if strings.Index(kv, "=") == -1 {
                        continue
                }
                parts := strings.SplitN(kv, "=", 2)
                if parts[0] != key {
                        continue
                }
                if len(parts) < 2 {
                        value = ""
                } else {
                        value = parts[1]
                }
        }
        return
}

func (job *Job) GetenvBool(key string) (value bool) {
	s := strings.ToLower(strings.Trim(job.Getenv(key), " \t"))
	if s == "" || s == "0" || s == "no" || s == "false" || s == "none" {
		return false
	}
	return true
}

func (job *Job) SetenvBool(key string, value bool) {
	if value {
		job.Setenv(key, "1")
	} else {
		job.Setenv(key, "0")
	}
}

func (job *Job) GetenvList(key string) []string {
	sval := job.Getenv(key)
	l := make([]string, 0, 1)
	if err := json.Unmarshal([]byte(sval), &l); err != nil {
		l = append(l, sval)
	}
	return l
}

func (job *Job) SetenvList(key string, value []string) error {
	sval, err := json.Marshal(value)
	if err != nil {
		return err
	}
	job.Setenv(key, string(sval))
	return nil
}

func (job *Job) Setenv(key, value string) {
	job.env = append(job.env, key + "=" + value)
}

// DecodeEnv decodes `src` as a json dictionary, and adds
// each decoded key-value pair to the environment.
//
// If `text` cannot be decoded as a json dictionary, an error
// is returned.
func (job *Job) DecodeEnv(src io.Reader) error {
	m := make(map[string]interface{})
	if err := json.NewDecoder(src).Decode(&m); err != nil {
		return err
	}
	for k, v := range m {
		if sval, ok := v.(string); ok {
			job.Setenv(k, sval)
		} else	if val, err := json.Marshal(v); err == nil {
			job.Setenv(k, string(val))
		} else {
			job.Setenv(k, fmt.Sprintf("%v", v))
		}
	}
	return nil
}

func (job *Job) EncodeEnv(dst io.Writer) error {
	m := make(map[string]interface{})
	for k, v := range job.Environ() {
		var val interface{}
		if err := json.Unmarshal([]byte(v), &val); err == nil {
			m[k] = val
		} else {
			m[k] = v
		}
	}
	if err := json.NewEncoder(dst).Encode(&m); err != nil {
		return err
	}
	return nil
}

func (job *Job) ExportEnv(dst interface{}) (err error) {
	var buf bytes.Buffer
	if err := job.EncodeEnv(&buf); err != nil {
		return err
	}
	if err := json.NewDecoder(&buf).Decode(dst); err != nil {
		return err
	}
	return nil
}

func (job *Job) ImportEnv(src interface{}) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(src); err != nil {
		return err
	}
	if err := job.DecodeEnv(&buf); err != nil {
		return err
	}
	return nil
}

func (job *Job) Environ() map[string]string {
	m := make(map[string]string)
	for _, kv := range job.env {
		parts := strings.SplitN(kv, "=", 2)
		m[parts[0]] = parts[1]
	}
	return m
}

func (job *Job) Logf(format string, args ...interface{}) (n int, err error) {
	prefixedFormat := fmt.Sprintf("[%s] %s\n", job, strings.TrimRight(format, "\n"))
	return fmt.Fprintf(job.Stdout, prefixedFormat, args...)
}
