package l2met

import (
	"bytes"
	"fmt"
	"log"
	"log/syslog"
	"net/http"
	"os"
	"text/template"
	"time"

	"io"
	"io/ioutil"
	"net"
	"net/url"
	"regexp"
	"sort"

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

	// Create the url
	query := ""
	if len(query) > 0 {
		queryString := buildQueryString(route.Options)
		query = fmt.Sprintf("?%s", queryString)
	}

	url := fmt.Sprintf("https://%s%s", route.Address, query)

	// Create the syslog RFC5424 template
	tmplStr := "<{{.Priority}}>1 {{.Timestamp}} {{.Container.Config.Hostname}} {{.ContainerName}} {{.Container.State.Pid}} - [] {{.Data}}\n"
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

		response, err := a.client.Do(request)
		if err != nil {
			log.Println("l2met:", err)
		}

		// Discard the response body so we can reuse the connection.
		io.Copy(ioutil.Discard, response.Body)
		response.Body.Close()
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
	return buf.Bytes(), nil
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

func buildQueryString(v map[string]string) string {
	var buf bytes.Buffer

	keys := make([]string, 0, len(v))

	for k := range v {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		vs := v[k]
		prefix := url.QueryEscape(k) + "="

		if buf.Len() > 0 {
			buf.WriteByte('&')
		}

		buf.WriteString(prefix)
		buf.WriteString(url.QueryEscape(vs))
	}

	return buf.String()
}

func dial(netw, addr string) (net.Conn, error) {
	dial, err := net.Dial(netw, addr)
	if err != nil {
		log.Println("l2met:", err)
	}

	return dial, err
}
