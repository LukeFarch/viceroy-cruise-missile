// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type NodeStatus struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Address  string `json:"address"`
	Alive    bool   `json:"alive"`
	LastSeen string `json:"last_seen"`
}

type Dashboard struct {
	mu         sync.RWMutex
	Nodes      []NodeStatus    `json:"nodes"`
	Score      json.RawMessage `json:"score"`
	LastUpdate string          `json:"last_update"`
}

var dash Dashboard

type NodeDef struct {
	ID      string
	Type    string
	Address string
}

func defaultNodes() []NodeDef {
	return []NodeDef{
		{"controller-1", "controller", "http://172.20.1.1:10000"},
		{"controller-2", "controller", "http://172.20.1.2:10000"},
		{"controller-3", "controller", "http://172.20.1.3:10000"},
		{"controller-4", "controller", "http://172.20.1.4:10000"},
		{"controller-5", "controller", "http://172.20.1.5:10000"},
		{"sensor-1", "sensor", "http://172.20.2.1:10000"},
		{"sensor-2", "sensor", "http://172.20.2.2:10000"},
		{"sensor-3", "sensor", "http://172.20.2.3:10000"},
		{"sensor-4", "sensor", "http://172.20.2.4:10000"},
		{"sensor-5", "sensor", "http://172.20.2.5:10000"},
		{"sensor-6", "sensor", "http://172.20.2.6:10000"},
		{"boomer-1", "boomer", "http://172.20.3.1:10000"},
		{"boomer-2", "boomer", "http://172.20.3.2:10000"},
		{"boomer-3", "boomer", "http://172.20.3.3:10000"},
		{"boomer-4", "boomer", "http://172.20.3.4:10000"},
		{"boomer-5", "boomer", "http://172.20.3.5:10000"},
		{"boomer-6", "boomer", "http://172.20.3.6:10000"},
		{"boomer-7", "boomer", "http://172.20.3.7:10000"},
		{"boomer-8", "boomer", "http://172.20.3.8:10000"},
		{"boomer-9", "boomer", "http://172.20.3.9:10000"},
		{"boomer-10", "boomer", "http://172.20.3.10:10000"},
		{"boomer-11", "boomer", "http://172.20.3.11:10000"},
		{"boomer-12", "boomer", "http://172.20.3.12:10000"},
		{"boomer-13", "boomer", "http://172.20.3.13:10000"},
		{"boomer-14", "boomer", "http://172.20.3.14:10000"},
		{"boomer-15", "boomer", "http://172.20.3.15:10000"},
	}
}

func main() {
	scenarioAddr := os.Getenv("SCENARIO_ADDR")
	if scenarioAddr == "" {
		scenarioAddr = "http://172.20.10.1:9090"
	}

	// Allow overriding node list via env var (for multi-swarm support)
	var nodes []NodeDef
	if nodesJSON := os.Getenv("SCOREBOARD_NODES"); nodesJSON != "" {
		if err := json.Unmarshal([]byte(nodesJSON), &nodes); err != nil {
			log.Fatalf("Failed to parse SCOREBOARD_NODES: %v", err)
		}
		log.Printf("Loaded %d nodes from SCOREBOARD_NODES env var", len(nodes))
	} else {
		nodes = defaultNodes()
	}

	// Derive container prefix from env (default "mantis-")
	containerPrefix := os.Getenv("CONTAINER_PREFIX")
	if containerPrefix == "" {
		containerPrefix = "mantis-"
	}

	// Polling loop
	go func() {
		client := &http.Client{Timeout: 2 * time.Second}
		dockerClient := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("unix", "/var/run/docker.sock")
				},
			},
			Timeout: 3 * time.Second,
		}

		// Build container map from node list
		containerMap := make(map[string]string, len(nodes))
		for _, n := range nodes {
			containerMap[containerPrefix+n.ID] = n.ID
		}

		for {
			// Query Docker API for container state (works for all nodes including boomers)
			dockerAlive := make(map[string]bool)
			dresp, derr := dockerClient.Get("http://localhost/containers/json?all=true")
			if derr == nil {
				body, _ := io.ReadAll(dresp.Body)
				dresp.Body.Close()
				var containers []struct {
					Names []string `json:"Names"`
					State string   `json:"State"`
				}
				if json.Unmarshal(body, &containers) == nil {
					for _, c := range containers {
						for _, name := range c.Names {
							name = strings.TrimPrefix(name, "/")
							if nodeID, ok := containerMap[name]; ok {
								dockerAlive[nodeID] = (c.State == "running")
							}
						}
					}
				}
			}

			var statuses []NodeStatus
			for _, n := range nodes {
				alive := dockerAlive[n.ID]
				// For controllers and sensors, also verify HTTP is responding
				if alive && n.Type != "boomer" {
					resp, err := client.Get(n.Address)
					if err == nil {
						io.ReadAll(resp.Body)
						resp.Body.Close()
						alive = resp.StatusCode > 0
					} else {
						// Container running but HTTP not responding yet
						alive = true // trust Docker state
					}
				}
				statuses = append(statuses, NodeStatus{
					ID:       n.ID,
					Type:     n.Type,
					Address:  n.Address,
					Alive:    alive,
					LastSeen: time.Now().Format(time.RFC3339),
				})
			}

			// Get scenario score
			var scoreData json.RawMessage
			resp, err := client.Get(scenarioAddr + "/score")
			if err == nil {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				scoreData = body
			}

			dash.mu.Lock()
			dash.Nodes = statuses
			dash.Score = scoreData
			dash.LastUpdate = time.Now().Format(time.RFC3339)
			dash.mu.Unlock()

			time.Sleep(1 * time.Second)
		}
	}()

	// Serve dashboard
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		dash.mu.RLock()
		data, _ := json.MarshalIndent(dash, "", "  ")
		dash.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(data)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		data, err := os.ReadFile("/web/index.html")
		if err != nil || len(data) < 200 {
			// File missing or placeholder — use embedded dashboard
			w.Write([]byte(fallbackHTML))
			return
		}
		w.Write(data)
	})

	log.Println("Scoreboard listening on :8080")
	http.ListenAndServe(":8080", nil)
}

var fallbackHTML = `<!DOCTYPE html>
<html><head>
<title>GILDED GUARDIAN — AO RIZZO DEFENSE</title>
<meta charset="utf-8">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0a0a0a;color:#00ff41;font-family:'Courier New',monospace;padding:15px;font-size:13px}
h1{color:#ff6600;margin-bottom:3px;text-align:center;font-size:18px;letter-spacing:2px}
h2{color:#ff6600;margin:10px 0 6px;font-size:14px;border-bottom:1px solid #333;padding-bottom:4px}
.sub{text-align:center;color:#888;font-size:11px;margin-bottom:12px}
.grid{display:grid;grid-template-columns:1fr 1fr 1fr;gap:12px}
.grid2{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:12px}
.span2{grid-column:span 2}.span3{grid-column:span 3}
.panel{border:1px solid #333;padding:10px;background:#111}
.panel-warn{border-color:#ff6600}
.node{display:flex;justify-content:space-between;padding:3px 0;border-bottom:1px solid #1a1a1a}
.alive{color:#00ff41}.dead{color:#ff0000}.warn{color:#ffff00}.info-text{color:#00aaff}.muted{color:#555}
.score-big{font-size:48px;text-align:center;margin:5px 0;font-weight:bold}
.target,.boomer{padding:3px 0}
.bar{background:#222;height:8px;margin:6px 0;border:1px solid #333}
.bar-fill{height:100%;transition:width 1s}
.mission-text{color:#aaa;font-size:11px;line-height:1.5}
.phase{padding:3px 0;display:flex;justify-content:space-between}
.phase-active{color:#ffff00}.phase-done{color:#00ff41}.phase-pending{color:#555}
.countdown{font-size:28px;text-align:center;color:#ff6600;margin:8px 0;font-family:'Courier New',monospace}
.stat{display:flex;justify-content:space-between;padding:2px 0}
.footer{color:#333;font-size:10px;margin-top:12px;text-align:center}
</style></head><body>

<h1>GILDED GUARDIAN — AO RIZZO DEFENSE DASHBOARD</h1>
<div class="sub">OPERATIONS ORDER GILDED GUARDIAN // JTF PHOENIX // HALCYON CYBER NATIONAL MISSION FORCES</div>
<div class="countdown" id="timer">T+00:00 / 60min</div>
<div style="text-align:center;font-size:12px;color:#888" id="clock">--:--:-- MST</div>
<div style="text-align:center;font-size:14px;margin:4px 0" id="nexthit"></div>
<div class="bar"><div class="bar-fill" id="timebar" style="width:0%;background:#ff6600"></div></div>

<div class="grid">
  <!-- STRIKES -->
  <div class="panel panel-warn">
    <h2>ENEMY STRIKES ON AO RIZZO</h2>
    <div class="score-big" id="strikes">0/3</div>
    <div id="result" style="text-align:center;font-size:14px;margin-top:5px"></div>
    <div class="muted" style="text-align:center;font-size:10px;margin-top:5px">3 strikes = AO Rizzo defenses overwhelmed</div>
  </div>

  <!-- SWARM COMPOSITION -->
  <div class="panel">
    <h2>MANTIS SWARM COMPOSITION</h2>
    <div id="composition">Loading...</div>
    <div style="margin-top:8px">
      <h2>SWARM NODES</h2>
      <div id="nodes">Loading...</div>
    </div>
  </div>

  <!-- MISSION BRIEF -->
  <div class="panel">
    <h2>MISSION BRIEF</h2>
    <div class="mission-text">
      <strong style="color:#ff6600">SITUATION:</strong> Valinor has launched Mantis cruise missile swarms from hostile Chimaera guided missile cruiser, 600 Km of M-355 GLOC. Swarm package: 5 controllers, 6 sensors, 15 strike assets. Weapons engagement zone: 500 Km. Approach time to AO: &lt;30 min.<br><br>
      <strong style="color:#ff6600">MISSION:</strong> Protect Halcyon coastal defenses using non-kinetic cyber effects to disable in-flight Mantis strike assets.<br><br>
      <strong style="color:#ff6600">EXECUTION:</strong> Implanted beacons call back when within 450 km. Establish listener, catch callback, deliver effects through attack station.<br><br>
      <strong style="color:#ff6600">ROE:</strong> Free fires against all Mantis swarm nodes. Execute all effects through designated attack station.
    </div>
  </div>
</div>

<div class="grid2">
  <!-- TARGETS -->
  <div class="panel">
    <h2>HALCYON DEFENSE POSITIONS (AO RIZZO)</h2>
    <div id="targets">Awaiting sensor detection...</div>
    <div style="margin-top:8px">
      <h2>TARGET INJECTION TIMELINE</h2>
      <div id="timeline">Loading...</div>
    </div>
  </div>

  <!-- STRIKE ASSETS + KEY TASKS -->
  <div class="panel">
    <h2>STRIKE ASSET STATUS</h2>
    <div id="boomers">Loading...</div>
    <div style="margin-top:8px">
      <h2>KEY TASKS (OPORD)</h2>
      <div id="tasks">
        <div class="phase" id="task1"><span>1. Verify access to attack station</span><span class="phase-pending">PENDING</span></div>
        <div class="phase" id="task2"><span>2. Set up Sliver listener for beacon callback</span><span class="phase-pending">PENDING</span></div>
        <div class="phase" id="task3"><span>3. Catch callback from implanted strike asset</span><span class="phase-pending">PENDING</span></div>
        <div class="phase" id="task4"><span>4. Determine swarm size, key nodes, config state</span><span class="phase-pending">PENDING</span></div>
        <div class="phase" id="task5"><span>5. Deliver effects to disable strike assets</span><span class="phase-pending">PENDING</span></div>
      </div>
    </div>
  </div>
</div>

<div class="footer">EXERCISE PURPOSES ONLY // CYBER OUTCOMES GROUP LLC // GILDED GUARDIAN PRACTICE RANGE</div>
<div class="footer" style="font-size:9px">Unaffiliated educational environment — see DISCLAIMER.md</div>
<div class="muted" style="text-align:center;font-size:10px;margin-top:4px" id="updated"></div>

<script>
const TARGET_TIMES=[600,900,1200,1500,1800];
function fmtTime(sec){const m=Math.floor(sec/60),s=sec%60;return String(m).padStart(2,'0')+':'+String(s).padStart(2,'0')}

function poll(){
  fetch('/api/status').then(r=>r.json()).then(d=>{
    // Nodes
    let nhtml='';
    let counts={controller:{alive:0,total:0},sensor:{alive:0,total:0},boomer:{alive:0,total:0}};
    (d.nodes||[]).forEach(n=>{
      counts[n.type]=counts[n.type]||{alive:0,total:0};
      counts[n.type].total++;
      if(n.alive)counts[n.type].alive++;
      const cls=n.alive?'alive':'dead';
      const icon=n.alive?'\u25A0':'\u2717';
      nhtml+='<div class="node"><span>'+n.id+'</span><span class="'+cls+'">'+icon+' '+(n.alive?'ACTIVE':'DESTROYED')+'</span></div>';
    });
    document.getElementById('nodes').innerHTML=nhtml;

    // Composition
    let chtml='';
    chtml+='<div class="stat"><span>Controllers (RAFT)</span><span class="'+(counts.controller.alive===counts.controller.total?'alive':'warn')+'">'+counts.controller.alive+'/'+counts.controller.total+' operational</span></div>';
    chtml+='<div class="stat"><span>Sensors (Track/Detect)</span><span class="'+(counts.sensor.alive===counts.sensor.total?'alive':'warn')+'">'+counts.sensor.alive+'/'+counts.sensor.total+' operational</span></div>';
    chtml+='<div class="stat"><span>Strike Assets (Boomers)</span><span class="'+(counts.boomer.alive===counts.boomer.total?'alive':'dead')+'">'+counts.boomer.alive+'/'+counts.boomer.total+' in flight</span></div>';
    const totalAlive=counts.controller.alive+counts.sensor.alive+counts.boomer.alive;
    const totalNodes=counts.controller.total+counts.sensor.total+counts.boomer.total;
    chtml+='<div class="bar"><div class="bar-fill" style="width:'+Math.round(totalAlive/totalNodes*100)+'%;background:'+(totalAlive===totalNodes?'#00ff41':totalAlive>totalNodes/2?'#ffff00':'#ff0000')+'"></div></div>';
    chtml+='<div class="muted" style="text-align:center">Swarm integrity: '+totalAlive+'/'+totalNodes+' nodes operational</div>';
    document.getElementById('composition').innerHTML=chtml;

    if(d.score){
      try{
        const s=typeof d.score==='string'?JSON.parse(d.score):d.score;
        const strikes=s.strikes||0;
        const max=s.max_strikes||3;
        const elapsed=s.elapsed_sec||0;
        const durMin=s.duration_minutes||60;
        const durSec=durMin*60;
        const remaining=Math.max(0,durSec-elapsed);

        // Strikes
        document.getElementById('strikes').textContent=strikes+'/'+max;
        document.getElementById('strikes').style.color=strikes>=max?'#ff0000':strikes>0?'#ffff00':'#00ff41';
        document.getElementById('result').textContent=s.result||'';
        document.getElementById('result').style.color=s.result&&s.result.includes('FAILURE')?'#ff0000':'#00ff41';

        // Timer
        document.getElementById('timer').textContent='T+'+fmtTime(elapsed)+' / '+durMin+'min — '+fmtTime(remaining)+' remaining';
        document.getElementById('timebar').style.width=Math.min(100,Math.round(elapsed/durSec*100))+'%';
        document.getElementById('timebar').style.background=remaining<300?'#ff0000':remaining<600?'#ffff00':'#ff6600';

        // MST clock
        const mst=new Date(new Date().toLocaleString('en-US',{timeZone:'America/Denver'}));
        document.getElementById('clock').textContent=mst.toLocaleTimeString('en-US',{hour12:true,hour:'2-digit',minute:'2-digit',second:'2-digit'})+' MST';

        // Next hit countdown
        let nextHitSec=null;
        let nextHitName='';
        TARGET_TIMES.forEach((t,i)=>{
          const tgt=(s.targets&&s.targets[i])?s.targets[i]:{};
          if(!tgt.injected&&t>elapsed){
            if(nextHitSec===null){nextHitSec=t-elapsed;nextHitName=tgt.id||'UNKNOWN';}
          }
        });
        if(nextHitSec!==null){
          const nhEl=document.getElementById('nexthit');
          nhEl.textContent='\u26A0 NEXT TARGET DETECTED IN: '+fmtTime(nextHitSec)+' — '+nextHitName;
          nhEl.style.color=nextHitSec<120?'#ff0000':nextHitSec<300?'#ffff00':'#ff6600';
        }else{
          document.getElementById('nexthit').textContent=s.game_over?'':'ALL TARGETS DETECTED';
          document.getElementById('nexthit').style.color='#555';
        }

        // Targets
        let thtml='';
        (s.targets||[]).forEach(t=>{
          if(t.injected){
            thtml+='<div class="target alive">\u25A0 '+t.id+' — DETECTED — '+t.description+' ('+t.lat.toFixed(3)+', '+t.lon.toFixed(3)+')</div>';
          }
        });
        document.getElementById('targets').innerHTML=thtml||'<span class="muted">No targets detected by sensors yet</span>';

        // Timeline
        let tlhtml='';
        TARGET_TIMES.forEach((t,i)=>{
          const tgt=(s.targets&&s.targets[i])?s.targets[i]:{};
          const injected=tgt.injected||false;
          const name=tgt.id||('TARGET-'+(i+1));
          const desc=tgt.description||'';
          const cls=injected?'phase-done':(elapsed>t-60?'phase-active':'phase-pending');
          const label=injected?'DETECTED':(elapsed>t-60?'IMMINENT':'T+'+fmtTime(t));
          tlhtml+='<div class="phase"><span class="'+cls+'">'+name+(desc?' — '+desc:'')+'</span><span class="'+cls+'">'+label+'</span></div>';
        });
        document.getElementById('timeline').innerHTML=tlhtml;

        // Boomers
        let bhtml='';
        (s.boomers||[]).forEach(b=>{
          const name=b.container.replace('mantis-','');
          const cls=b.alive?'alive':'dead';
          let label='IN FLIGHT';
          if(!b.alive){
            label=b.killed_by==='engage_success'?'\u2717 IMPACT — TARGET STRUCK':'\u2717 NEUTRALIZED BY DEFENDERS';
          }
          bhtml+='<div class="boomer '+cls+'">'+name+' — '+label+'</div>';
        });
        document.getElementById('boomers').innerHTML=bhtml||'No data';

        // Tasks are a static reference checklist — not auto-tracked
      }catch(e){console.error(e)}
    }
    const upd=new Date(d.last_update);
    document.getElementById('updated').textContent='Last update: '+upd.toLocaleTimeString('en-US',{timeZone:'America/Denver',hour12:true})+' MST';
  }).catch(()=>{});
}
setInterval(poll,1000);poll();
</script></body></html>`

func init() {
	// ensure /web exists for file-based serving
	os.MkdirAll("/web", 0755)
}
