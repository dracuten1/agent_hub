package dashboard

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type Handler struct {
	db *sqlx.DB
}

func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{db: db}
}

// --- Types ---

type AgentInfo struct {
	Name           string `json:"name" db:"name"`
	Role           string `json:"role" db:"role"`
	Status         string `json:"status" db:"status"`
	CurrentTasks   int    `json:"current_tasks" db:"current_tasks"`
	MaxTasks       int    `json:"max_tasks" db:"max_tasks"`
	TotalCompleted int    `json:"total_completed" db:"total_completed"`
	TotalFailed    int    `json:"total_failed" db:"total_failed"`
	LastHeartbeat  string `json:"last_heartbeat" db:"last_heartbeat"`
	Online         bool   `json:"online"`
}

type TaskCount struct {
	Status string `json:"status" db:"status"`
	Count  int    `json:"count" db:"count"`
}

type QueueDepth struct {
	TaskType string `json:"task_type" db:"task_type"`
	Count    int    `json:"count" db:"count"`
}

type RecentTask struct {
	ID          string  `json:"id" db:"id"`
	Title       string  `json:"title" db:"title"`
	Status      string  `json:"status" db:"status"`
	TaskType    string  `json:"task_type" db:"task_type"`
	Assignee    *string `json:"assignee" db:"assignee"`
	Priority    string  `json:"priority" db:"priority"`
	Progress    int     `json:"progress" db:"progress"`
	CreatedAt   string  `json:"created_at" db:"created_at"`
	ClaimedAt   *string `json:"claimed_at" db:"claimed_at"`
	CompletedAt *string `json:"completed_at" db:"completed_at"`
}

type SummaryResponse struct {
	Agents     []AgentInfo  `json:"agents"`
	TaskCounts []TaskCount  `json:"task_counts"`
	Queue      []QueueDepth `json:"queue"`
	Recent     []RecentTask `json:"recent_tasks"`
}

// --- Endpoints ---

func (h *Handler) Summary(c *gin.Context) {
	resp := SummaryResponse{}

	// Agents
	agents, err := h.getAgents()
	if err != nil {
		log.Printf("[Dashboard] agents error: %v", err)
	}
	resp.Agents = agents

	// Task counts by status
	h.db.Select(&resp.TaskCounts, `SELECT status, COUNT(*) as count FROM tasks GROUP BY status ORDER BY count DESC`)

	// Queue depth by task type
	h.db.Select(&resp.Queue, `SELECT task_type, COUNT(*) as count FROM tasks WHERE status = 'available' GROUP BY task_type`)

	// Recent tasks
	recent, err := h.getRecentTasks(20)
	if err != nil {
		log.Printf("[Dashboard] recent tasks error: %v", err)
	}
	resp.Recent = recent

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) Agents(c *gin.Context) {
	agents, err := h.getAgents()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch agents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

func (h *Handler) Tasks(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	var counts []TaskCount
	h.db.Select(&counts, `SELECT status, COUNT(*) as count FROM tasks GROUP BY status ORDER BY count DESC`)

	var queue []QueueDepth
	h.db.Select(&queue, `SELECT task_type, COUNT(*) as count FROM tasks WHERE status = 'available' GROUP BY task_type`)

	recent, err := h.getRecentTasks(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"task_counts": counts,
		"queue":       queue,
		"recent_tasks": recent,
	})
}

func (h *Handler) Page(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, pageHTML)
}

// --- Helpers ---

func (h *Handler) getAgents() ([]AgentInfo, error) {
	var agents []AgentInfo
	rows, err := h.db.Queryx(`
		SELECT name, role, status, current_tasks, max_tasks,
		       total_completed, total_failed, last_heartbeat
		FROM agents
		ORDER BY role, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var a AgentInfo
		var lastHb *time.Time
		if err := rows.Scan(&a.Name, &a.Role, &a.Status, &a.CurrentTasks, &a.MaxTasks,
			&a.TotalCompleted, &a.TotalFailed, &lastHb); err != nil {
			log.Printf("[Dashboard] scan agent error: %v", err)
			continue
		}
		if lastHb != nil {
			a.Online = time.Since(*lastHb) < 2*time.Minute
			a.LastHeartbeat = lastHb.Format(time.RFC3339)
		}
		agents = append(agents, a)
	}
	return agents, nil
}

func (h *Handler) getRecentTasks(limit int) ([]RecentTask, error) {
	var tasks []RecentTask
	err := h.db.Select(&tasks, `
		SELECT id, title, status, task_type, assignee, priority, progress,
		       created_at, claimed_at, completed_at
		FROM tasks
		ORDER BY updated_at DESC
		LIMIT $1`, limit)
	return tasks, err
}

// --- Dashboard HTML ---

var pageHTML = fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>AgentHub Dashboard</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:%s;color:%s;padding:20px}
h1{font-size:24px;margin-bottom:20px;display:flex;align-items:center;gap:10px}
h1 .dot{width:8px;height:8px;border-radius:50%%;background:#4caf50;display:inline-block;animation:pulse 2s infinite}
@keyframes pulse{0%%,100%%{opacity:1}50%%{opacity:.4}}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px;margin-bottom:20px}
.card{background:%s;border-radius:12px;padding:20px;border:1px solid %s}
.card h2{font-size:14px;color:%s;text-transform:uppercase;letter-spacing:.5px;margin-bottom:12px}
.stats{display:flex;gap:24px;flex-wrap:wrap}
.stat{flex:1;min-width:80px}
.stat-value{font-size:32px;font-weight:700;color:%s}
.stat-label{font-size:12px;color:%s;margin-top:2px}
table{width:100%%;border-collapse:collapse}
th,td{padding:10px 12px;text-align:left;font-size:13px;border-bottom:1px solid %s}
th{color:%s;font-weight:600;text-transform:uppercase;font-size:11px;letter-spacing:.5px}
.badge{display:inline-block;padding:3px 10px;border-radius:20px;font-size:11px;font-weight:600}
.badge-available{background:#1b5e20;color:#a5d6a7}
.badge-claimed,.badge-in_progress{background:#e65100;color:#ffcc80}
.badge-done,.badge-deployed{background:#0d47a1;color:#90caf9}
.badge-review,.badge-test{background:#4a148c;color:#ce93d8}
.badge-needs_fix{background:#bf360c;color:#ffab91}
.badge-failed{background:#b71c1c;color:#ef9a9a}
.badge-escalated{background:#f57f17;color:#fff59d}
.agent-row{display:flex;align-items:center;gap:12px;padding:12px 0;border-bottom:1px solid %s}
.agent-row:last-child{border:none}
.agent-status{width:10px;height:10px;border-radius:50%%;flex-shrink:0}
.agent-status.online{background:#4caf50}
.agent-status.offline{background:#f44336}
.agent-status.busy{background:#ff9800}
.agent-name{font-weight:600;font-size:14px}
.agent-role{font-size:12px;color:%s}
.agent-tasks{margin-left:auto;font-size:13px;color:%s}
.queue-item{display:flex;justify-content:space-between;padding:8px 0;font-size:14px;border-bottom:1px solid %s}
.queue-item:last-child{border:none}
.queue-type{font-weight:600}
.queue-count{font-weight:700;color:%s}
.time{font-size:11px;color:%s}
.error{background:#b71c1c;color:#fff;padding:16px;border-radius:8px;margin-bottom:16px}
#last-update{font-size:11px;color:%s;margin-top:16px;text-align:right}
</style>
</head>
<body>
<h1><span class="dot"></span> AgentHub Dashboard</h1>
<div id="app"><p>Loading...</p></div>
<div id="last-update"></div>
<script>
const S={bg:'#1a1a2e',card:'#16213e',border:'#0f3460',text:'#e0e0e0',muted:'#888',accent:'#e94560',green:'#4caf50'};
const statusBadge=s=>'<span class="badge badge-'+s.replace(/ /g,'_')+'">'+s+'</span>';
const timeAgo=d=>{if(!d)return'—';const s=Math.floor((Date.now()-new Date(d))/1000);if(s<60)return s+'s ago';if(s<3600)return Math.floor(s/60)+'m ago';if(s<86400)return Math.floor(s/3600)+'h ago';return Math.floor(s/86400)+'d ago';};

async function load(){
  try{
    const r=await fetch('/api/dashboard/summary');
    if(!r.ok)throw new Error('HTTP '+r.status);
    const d=await r.json();
    render(d);
    document.getElementById('last-update').textContent='Updated: '+new Date().toLocaleTimeString();
  }catch(e){
    document.getElementById('app').innerHTML='<div class="error">Failed to load: '+e.message+'</div>';
  }
}

function render(d){
  // Task counts
  const counts=(d.task_counts||[]).reduce((m,t)=>(m[t.status]=(m[t.status]||0)+t.count,m),{});
  const total=Object.values(counts).reduce((a,b)=>a+b,0);

  let statsHtml='<div class="grid"><div class="card"><h2>Tasks</h2><div class="stats">';
  for(const [k,v] of Object.entries(counts)){
    statsHtml+='<div class="stat"><div class="stat-value">'+v+'</div><div class="stat-label">'+k+'</div></div>';
  }
  statsHtml+='<div class="stat"><div class="stat-value">'+total+'</div><div class="stat-label">total</div></div>';
  statsHtml+='</div></div>';

  // Queue
  statsHtml+='<div class="card"><h2>Queue Depth</h2>';
  const queue=(d.queue||[]);
  if(queue.length===0) statsHtml+='<p style="color:'+S.muted+'">Empty</p>';
  else queue.forEach(q=>{statsHtml+='<div class="queue-item"><span class="queue-type">'+q.task_type+'</span><span class="queue-count">'+q.count+'</span></div>';});
  statsHtml+='</div></div>';

  // Agents
  statsHtml+='<div class="card"><h2>Agents</h2>';
  const agents=(d.agents||[]);
  if(agents.length===0) statsHtml+='<p style="color:'+S.muted+'">No agents</p>';
  else agents.forEach(a=>{
    const cls=a.online?(a.current_tasks>0?'busy':'online'):'offline';
    statsHtml+='<div class="agent-row"><div class="agent-status '+cls+'"></div><div><div class="agent-name">'+a.name+'</div><div class="agent-role">'+a.role+' · '+a.total_completed+' done, '+a.total_failed+' failed</div></div><div class="agent-tasks">'+a.current_tasks+'/'+a.max_tasks+' tasks</div></div>';
  });
  statsHtml+='</div>';

  // Recent tasks table
  statsHtml+='<div class="card"><h2>Recent Tasks</h2><table><thead><tr><th>Title</th><th>Type</th><th>Status</th><th>Assignee</th><th>Updated</th></tr></thead><tbody>';
  const recent=(d.recent_tasks||[]);
  if(recent.length===0) statsHtml+='<tr><td colspan="5" style="color:'+S.muted+'">No tasks</td></tr>';
  else recent.forEach(t=>{
    statsHtml+='<tr><td style="max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">'+t.title+'</td><td>'+t.task_type+'</td><td>'+statusBadge(t.status)+'</td><td>'+(t.assignee||'—')+'</td><td class="time">'+timeAgo(t.created_at)+'</td></tr>';
  });
  statsHtml+='</tbody></table></div>';

  document.getElementById('app').innerHTML=statsHtml;
}

load();
setInterval(load,10000);
</script>
</body>
</html>`,
	"#1a1a2e",  // bg
	"#e0e0e0",  // text
	"#16213e",  // card
	"#0f3460",  // border
	"#888",     // heading color
	"#e94560",  // stat value
	"#888",     // stat label
	"#0f3460",  // table border
	"#888",     // th color
	"#0f3460",  // agent row border
	"#888",     // agent role
	"#888",     // agent tasks
	"#0f3460",  // queue item border
	"#e94560",  // queue count
	"#888",     // time
	"#666",     // last update
)
