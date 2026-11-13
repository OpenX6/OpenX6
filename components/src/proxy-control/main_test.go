package main
import "testing"
func sample(status string,ms int64)media{var m media;m.Record.Status=status;m.Record.Time=ms;return m}
func TestPolicy(t *testing.T){
 var p policy
 if p.want(true,sample("RecordIdle",0))||p.want(true,sample("RecordStarting",0)){t.Fatal("idle/start mutated")}
 if p.want(false,sample("RecordRunning",1000)){t.Fatal("off stopped")}
 if !p.want(true,sample("RecordRunning",3000)){t.Fatal("enabled did not stop")};p.stopped=true
 if p.want(true,sample("RecordRunning",5000)){t.Fatal("repeat stop")}
 if !p.want(true,sample("RecordRunning",500)){t.Fatal("new recording not detected")};p.stopped=true
 p.want(true,sample("RecordIdle",0));if !p.want(true,sample("RecordRunning",200)){t.Fatal("restart")}
 p.attempts=3;if p.want(true,sample("RecordRunning",400)){t.Fatal("unbounded retry")}
}
func TestInvalidPreference(t *testing.T){if save("/nonexistent/state",2)==nil{t.Fatal("invalid preference accepted")}}
