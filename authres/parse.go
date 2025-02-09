package authres

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

)

// ResultValue is an authentication result value, as defined in RFC 5451 section
// 6.3.
type ResultValue string

const (
	ResultNone      ResultValue = "none"
	ResultPass                  = "pass"
	ResultFail                  = "fail"
	ResultPolicy                = "policy"
	ResultNeutral               = "neutral"
	ResultTempError             = "temperror"
	ResultPermError             = "permerror"
	ResultHardFail              = "hardfail"
	ResultSoftFail              = "softfail"
)

type Parsed struct {
	Identifier string
	Instance int
	Results []Result
	Error error
}

// Result is an authentication result.
type Result interface {
	parse(value ResultValue, params map[string]string)
	format() (value ResultValue, params map[string]string)
}

type AuthResult struct {
	Value  ResultValue
	Reason string
	Auth   string
}

func (r *AuthResult) parse(value ResultValue, params map[string]string) {
	r.Value = value
	r.Reason = params["reason"]
	r.Auth = params["smtp.auth"]
}

func (r *AuthResult) format() (ResultValue, map[string]string) {
	return r.Value, map[string]string{"smtp.auth": r.Auth}
}

type DKIMResult struct {
	Value      ResultValue
	Reason     string
	Domain     string
	Identifier string
}

func (r *DKIMResult) parse(value ResultValue, params map[string]string) {
	r.Value = value
	r.Reason = params["reason"]
	r.Domain = params["header.d"]
	r.Identifier = params["header.i"]
}

func (r *DKIMResult) format() (ResultValue, map[string]string) {
	return r.Value, map[string]string{
		"reason":   r.Reason,
		"header.d": r.Domain,
		"header.i": r.Identifier,
	}
}

type DomainKeysResult struct {
	Value  ResultValue
	Reason string
	Domain string
	From   string
	Sender string
}

func (r *DomainKeysResult) parse(value ResultValue, params map[string]string) {
	r.Value = value
	r.Reason = params["reason"]
	r.Domain = params["header.d"]
	r.From = params["header.from"]
	r.Sender = params["header.sender"]
}

func (r *DomainKeysResult) format() (ResultValue, map[string]string) {
	return r.Value, map[string]string{
		"reason":        r.Reason,
		"header.d":      r.Domain,
		"header.from":   r.From,
		"header.sender": r.Sender,
	}
}

type IPRevResult struct {
	Value  ResultValue
	Reason string
	IP     string
}

func (r *IPRevResult) parse(value ResultValue, params map[string]string) {
	r.Value = value
	r.Reason = params["reason"]
	r.IP = params["policy.iprev"]
}

func (r *IPRevResult) format() (ResultValue, map[string]string) {
	return r.Value, map[string]string{
		"reason":       r.Reason,
		"policy.iprev": r.IP,
	}
}

type SenderIDResult struct {
	Value       ResultValue
	Reason      string
	HeaderKey   string
	HeaderValue string
}

func (r *SenderIDResult) parse(value ResultValue, params map[string]string) {
	r.Value = value
	r.Reason = params["reason"]

	for k, v := range params {
		if strings.HasPrefix(k, "header.") {
			r.HeaderKey = strings.TrimPrefix(k, "header.")
			r.HeaderValue = v
			break
		}
	}
}

func (r *SenderIDResult) format() (value ResultValue, params map[string]string) {
	return r.Value, map[string]string{
		"reason":                                 r.Reason,
		"header." + strings.ToLower(r.HeaderKey): r.HeaderValue,
	}
}

type SPFResult struct {
	Value  ResultValue
	Reason string
	From   string
	Helo   string
}

func (r *SPFResult) parse(value ResultValue, params map[string]string) {
	r.Value = value
	r.Reason = params["reason"]
	r.From = params["smtp.mailfrom"]
	r.Helo = params["smtp.helo"]
}

func (r *SPFResult) format() (ResultValue, map[string]string) {
	return r.Value, map[string]string{
		"reason":        r.Reason,
		"smtp.mailfrom": r.From,
		"smtp.helo":     r.Helo,
	}
}

type DMARCResult struct {
	Value  ResultValue
	Reason string
	From   string
}

func (r *DMARCResult) parse(value ResultValue, params map[string]string) {
	r.Value = value
	r.Reason = params["reason"]
	r.From = params["header.from"]
}

func (r *DMARCResult) format() (ResultValue, map[string]string) {
	return r.Value, map[string]string{
		"reason":      r.Reason,
		"header.from": r.From,
	}
}

type GenericResult struct {
	Method string
	Value  ResultValue
	Params map[string]string
}

func (r *GenericResult) parse(value ResultValue, params map[string]string) {
	r.Value = value
	r.Params = params
}

func (r *GenericResult) format() (ResultValue, map[string]string) {
	return r.Value, r.Params
}

type newResultFunc func() Result

var results = map[string]newResultFunc{
	"auth": func() Result {
		return new(AuthResult)
	},
	"dkim": func() Result {
		return new(DKIMResult)
	},
	"domainkeys": func() Result {
		return new(DomainKeysResult)
	},
	"iprev": func() Result {
		return new(IPRevResult)
	},
	"sender-id": func() Result {
		return new(SenderIDResult)
	},
	"spf": func() Result {
		return new(SPFResult)
	},
	"dmarc": func() Result {
		return new(DMARCResult)
	},
}

// Parse parses the provided Authentication-Results header field. It returns the
// authentication service identifier and authentication results.
func Parse(v string) *Parsed {
	var parResults []Result
	parsed := &Parsed{}
	parts := strings.Split(v, ";")
	start := 1
	parsed.Identifier = strings.TrimSpace(parts[0])
	if strings.HasPrefix(parsed.Identifier, "i=") {
		// We are dealing with ARC-Authentication-Results
		// https://www.rfc-editor.org/rfc/rfc8617.html#section-4.2.1
		// Let's make sure
		kv := strings.SplitN(parsed.Identifier, "=", 2)
		if len(kv) == 2 {
			ins, err := strconv.Atoi(kv[1])
			// Instance tag values can range from 1-50 (inclusive).
			if err == nil && ins > 0 && ins <= 50  {
				parsed.Instance = ins
				parsed.Identifier = strings.TrimSpace(parts[1])
				start = 2
			}
		}
	}
	i := strings.IndexFunc(parsed.Identifier, unicode.IsSpace)
	if i > 0 {
		// Authentication-Results: example.org 1;
		version := strings.TrimSpace(parsed.Identifier[i:])
		if version != "1" {
			parsed.Identifier = ""
			parsed.Results = nil
			parsed.Error = errors.New("msgauth: unsupported version")
			return parsed
		}

		parsed.Identifier = parsed.Identifier[:i]
	}


	for i := start; i < len(parts); i++ {
		s := strings.TrimSpace(parts[i])
		if s == "" {
			continue
		}

		result, err := parseResult(s)
		if err != nil {
			parsed.Results = parResults
			parsed.Error = err
			return parsed
		}
		if result != nil {
			parResults = append(parResults, result)
			parsed.Results = parResults
		}
	}
	return parsed
}

func parseResult(s string) (Result, error) {
	// TODO: ignore header comments in parenthesis

	parts := strings.Fields(s)
	if len(parts) == 0 || parts[0] == "none" {
		return nil, nil
	}

	k, v, err := parseParam(parts[0])
	if err != nil {
		return nil, err
	}
	method, value := k, ResultValue(strings.ToLower(v))

	params := make(map[string]string)
	for i := 1; i < len(parts); i++ {
		k, v, err := parseParam(parts[i])
		if err != nil {
			continue
		}

		params[k] = v
	}

	newResult, ok := results[method]

	var r Result
	if ok {
		r = newResult()
	} else {
		r = &GenericResult{
			Method: method,
			Value:  value,
			Params: params,
		}
	}

	r.parse(value, params)
	return r, nil
}

func parseParam(s string) (k string, v string, err error) {
	kv := strings.SplitN(s, "=", 2)
	if len(kv) != 2 {
		return "", "", errors.New("msgauth: malformed authentication method and value")
	}
	return strings.ToLower(strings.TrimSpace(kv[0])), strings.TrimSpace(kv[1]), nil
}
