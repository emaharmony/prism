package main
import ("encoding/json";"fmt";"os";"time";"github.com/nats-io/nats.go")
func main(){
 url:=os.Args[1];target:=os.Args[2];taskID:=os.Args[3];desc:=os.Args[4]
 if len(desc)>1&&desc[0]=='@'{b,e:=os.ReadFile(desc[1:]);if e!=nil{fmt.Println("read err:",e);os.Exit(1)};desc=string(b)}
 nc,e:=nats.Connect(url);if e!=nil{fmt.Println("connect err:",e);os.Exit(1)};defer nc.Close()
 pkt:=map[string]any{"type":"task_delegation","target_agent":target,"task_id":taskID,"description":desc,"expected_deliverable":"per description","priority":"normal","required_capability":"code"}
 done:=make(chan string,1)
 nc.Subscribe("prism.workflow.task.complete",func(m *nats.Msg){var c map[string]any;if json.Unmarshal(m.Data,&c)==nil&&c["task_id"]==taskID{done<-string(m.Data)}})
 d,_:=json.Marshal(pkt);nc.Publish("prism.agent.openclaw",d);nc.Flush()
 fmt.Println("PUBLISHED "+target+" "+taskID)
 select{case r:=<-done:fmt.Println("COMPLETION: "+r);case <-time.After(12*time.Minute):fmt.Println("TIMEOUT")}
}
