package l2met

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"log/syslog"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"text/template"
	"time"

	"github.com/gliderlabs/logspout/router"
)

var hostname string

func init() {
	hostname, _ = os.Hostname()

	router.AdapterFactories.Register(NewL2metAdapter, "l2met")
}

// L2metAdapter is an adapter that streams HTTPS RFC5424 to l2met.
type L2metAdapter struct {
	client *http.Client
	url    string
	route  *router.Route
	tmpl   *template.Template
}

// NewL2metAdapter creates a L2metAdapter with HTTPS as its transport.
func NewL2metAdapter(route *router.Route) (router.LogAdapter, error) {
	// Create the client
	transport := &http.Transport{}
	transport.Dial = dial

	client := &http.Client{Transport: transport}

	// Get the url
	url := os.Getenv("L2MET_URL")
	if url == "" {
		return nil, errors.New("l2met url not found")
	}

	// Create the syslog RFC5424 template
	tmplStr := "<{{.Priority}}>1 {{.Timestamp}} {{.Container.Config.Hostname}} {{.ContainerName}} {{.Container.State.Pid}} - - {{.Data}}\n"
	tmpl, err := template.New("l2met").Parse(tmplStr)
	if err != nil {
		return nil, err
	}

	return &L2metAdapter{
		route:  route,
		client: client,
		url:    url,
		tmpl:   tmpl,
	}, nil
}

func (a *L2metAdapter) Stream(logStream chan *router.Message) {
	for message := range logStream {
		// Check if this message is a l2met message, otherwise continue.
		if match, _ := regexp.MatchString("(measure|sample|count)#.+=.+", message.Data); !match {
			continue
		}

		m := &SyslogMessage{message}

		buf, err := m.Render(a.tmpl)
		if err != nil {
			log.Println("l2met:", err)
			return
		}

		body := bytes.NewReader(buf)
		request, err := http.NewRequest("POST", a.url, body)
		if err != nil {
			log.Println("l2met:", err)
		}

		go func(request *http.Request) {
			response, err := a.client.Do(request)
			if err != nil {
				log.Println("l2met:", err)
			}

			if response.StatusCode != 200 {
				log.Println("l2met: Error sending log ", response.StatusCode)
			}

			// Discard the response body so we can reuse the connection.
			io.Copy(ioutil.Discard, response.Body)
			response.Body.Close()
		}(request)
	}
}

// This is taken from Logspout's syslog adapter.
type SyslogMessage struct {
	*router.Message
}

func (m *SyslogMessage) Render(tmpl *template.Template) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := tmpl.Execute(buf, m)
	if err != nil {
		return nil, err
	}

	// We need to length prefix this
	length := strconv.Itoa(buf.Len())

	return bytes.Join([][]byte{[]byte(length), buf.Bytes()}, []byte(" ")), nil
}

func (m *SyslogMessage) Priority() syslog.Priority {
	switch m.Message.Source {
	case "stdout":
		return syslog.LOG_USER | syslog.LOG_INFO
	case "stderr":
		return syslog.LOG_USER | syslog.LOG_ERR
	default:
		return syslog.LOG_DAEMON | syslog.LOG_INFO
	}
}

func (m *SyslogMessage) Hostname() string {
	return hostname
}

func (m *SyslogMessage) Timestamp() string {
	return m.Message.Time.Format(time.RFC3339)
}

func (m *SyslogMessage) ContainerName() string {
	return m.Message.Container.Name[1:]
}

func dial(netw, addr string) (net.Conn, error) {
	dial, err := net.Dial(netw, addr)
	if err != nil {
		log.Println("l2met:", err)
	}

	return dial, err
}
