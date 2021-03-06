package jenkobs_reactor

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/jinzhu/copier"
)

/*
	HTTP Action calls specified URL if criterion meets
*/

// HTTPAction object
type HTTPAction struct {
	auth *JenkinsAuth
	BaseAction
}

// NewHTTPAction constructor
func NewHTTPAction() *HTTPAction {
	hta := new(HTTPAction)
	return hta
}

// Get URL for the action. If the URL is without scheme,
// then it will be treated as FQDN-less URN, and a default hostname will be used.
func (hta *HTTPAction) getURL() string {
	jkFqdn := hta.auth.Fqdn
	if jkFqdn == "" {
		jkFqdn = "localhost"
	}
	jkPort := hta.auth.Port

	url, ok := hta.actionInfo.Params["query"].(map[string]interface{})["url"]
	if !ok {
		hta.GetLogger().Errorln("URL is not defined to the HTTP action. Check your configuration.")
		return ""
	}

	out := url.(string)
	if !strings.Contains(out, "://") {
		var buff strings.Builder
		buff.WriteString("https://")
		buff.WriteString(jkFqdn)
		if jkPort != 443 && jkPort != 0 {
			buff.WriteString(":" + strconv.Itoa(jkPort))
		}
		buff.WriteString("/" + strings.TrimPrefix(path.Clean(out), "/"))
		out = buff.String()
	}

	return out
}

// SetJenkinsAuth sets the authentication data for Jenkins
func (hta *HTTPAction) SetJenkinsAuth(auth *JenkinsAuth) {
	hta.auth = auth
}

// Perform a HTTP/S request
func (hta *HTTPAction) request(message *ReactorDelivery) error {
	query, ok := hta.GetActionInfo().Params["query"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("Query is not defined for the HTTP action watching '%s' project", hta.GetActionInfo().Project)
	}

	method, ok := query["method"].(string)
	if !ok {
		method = "get"
	}

	params := hta.actionInfo.Params["query"].(map[string]interface{})["params"]
	data := url.Values{}
	for qkey, qval := range params.(map[string]interface{}) {
		if qkey == "" {
			continue
		} else if qval != nil {
			data.Add(qkey, qval.(string))
		} else {
			data.Add(qkey, "")
		}
	}

	switch strings.ToLower(method) {
	case "post":
		request, err := http.NewRequest("POST", hta.getURL(), strings.NewReader(data.Encode()))
		if err != nil {
			return err
		}
		request.SetBasicAuth(hta.auth.User, hta.auth.Token)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(request)
		if err != nil {
			return err
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		hta.GetLogger().Debugln(string(body))
	case "get":
	default:
		return fmt.Errorf("Method %s is not supported in HTTP action", strings.ToUpper(method))
	}

	return nil
}

// MakeActionInstance creates a self-contained instance copy
func (hta *HTTPAction) MakeActionInstance() interface{} {
	action := NewHTTPAction()
	src := *hta.actionInfo
	dst := &ActionInfo{}
	if err := copier.Copy(&dst, &src); err != nil {
		hta.GetLogger().Errorf("Error initialising HTTP call instance: %s", err.Error())
	}
	action.actionInfo = dst
	action.SetJenkinsAuth(hta.auth)

	return *action
}

// OnMessage when delivery comes, match the criteria according to the action info
func (hta HTTPAction) OnMessage(message *ReactorDelivery) error {
	base := hta.MakeActionInstance().(HTTPAction)
	if !message.IsValid() {
		return fmt.Errorf("Skipping invalid message")
	}

	if base.Matches(message) {
		return base.request(message)
	}

	return nil
}
