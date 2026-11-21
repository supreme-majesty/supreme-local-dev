package server

import "testing"

func TestParseAccessLog(t *testing.T) {
	// Lines as Traefik v3.6 writes them with orchestrate.RouterStaticConfig.
	routed := `{"ClientAddr":"127.0.0.1:56360","ClientHost":"127.0.0.1","DownstreamContentSize":19,"DownstreamStatus":404,"Duration":6200874,"OriginDuration":6103141,"OriginStatus":404,"RequestHost":"t.localhost","RequestMethod":"GET","RequestPath":"/hello?x=1","RequestScheme":"http","RouterName":"t@file","StartUTC":"2026-11-21T03:23:48.727348384Z","entryPointName":"web","level":"info","msg":"","request_User-Agent":"probe-agent","time":"2026-11-21T03:23:48Z"}`
	internal := `{"ClientHost":"127.0.0.1","DownstreamStatus":404,"Duration":96967,"RequestHost":"t.localhost","RequestMethod":"GET","RequestPath":"/hello?x=1","StartUTC":"2026-11-21T03:23:48.729292538Z","entryPointName":"traefik","level":"info"}`
	traefikLog := `{"level":"error","error":"x","time":"2026-11-21T03:23:30Z","message":"Command error"}`

	e, ok := parseAccessLog([]byte(routed))
	if !ok {
		t.Fatal("routed request not parsed")
	}
	if e.Host != "t.localhost" || e.Method != "GET" || e.URI != "/hello?x=1" || e.Status != 404 || e.BodyBytes != 19 ||
		e.Agent != "probe-agent" || e.Scheme != "http" || e.RemoteAddr != "127.0.0.1" {
		t.Errorf("entry = %+v", e)
	}
	if e.Latency != "0.006201" || e.UpstreamLatency != "0.006103" {
		t.Errorf("latency = %s / %s", e.Latency, e.UpstreamLatency)
	}
	for name, line := range map[string]string{"internal hop": internal, "traefik log": traefikLog, "text": "time=x level=info", "empty": ""} {
		if _, ok := parseAccessLog([]byte(line)); ok {
			t.Errorf("%s must be skipped", name)
		}
	}
}
