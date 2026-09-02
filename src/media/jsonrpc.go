package media

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

type JsonRpcRequest struct {
	JsonRpc string        `json:"jsonrpc"`
	Id      interface{}   `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type JsonRpcResponse struct {
	JsonRpc string      `json:"jsonrpc"`
	Id      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// Aria2JsonRpcHandler handles /jsonrpc and /rpc requests from browser extensions (Ghost Downloader, Aria2 integration, etc.)
func Aria2JsonRpcHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil || len(bodyBytes) == 0 {
		http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"}}`, http.StatusOK)
		return
	}

	// Handle batch or single request
	if strings.HasPrefix(strings.TrimSpace(string(bodyBytes)), "[") {
		var reqs []JsonRpcRequest
		if err := json.Unmarshal(bodyBytes, &reqs); err != nil {
			http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"}}`, http.StatusOK)
			return
		}
		var responses []JsonRpcResponse
		for _, req := range reqs {
			responses = append(responses, handleSingleAria2Request(req))
		}
		_ = json.NewEncoder(w).Encode(responses)
		return
	}

	var req JsonRpcRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"}}`, http.StatusOK)
		return
	}

	res := handleSingleAria2Request(req)
	_ = json.NewEncoder(w).Encode(res)
}

func handleSingleAria2Request(req JsonRpcRequest) JsonRpcResponse {
	res := JsonRpcResponse{
		JsonRpc: "2.0",
		Id:      req.Id,
	}

	switch req.Method {
	case "aria2.getVersion", "system.version":
		res.Result = map[string]interface{}{
			"version":         "1.37.0",
			"enabledFeatures": []string{"BitTorrent", "GZip", "HTTPS", "MessageDigest", "Firefox3Cookie", "Async DNS"},
		}

	case "aria2.getGlobalStat":
		tasks := GlobalQueue.GetTasks()
		activeCount := 0
		queuedCount := 0
		for _, t := range tasks {
			if t.Status == StatusDownloading {
				activeCount++
			} else if t.Status == StatusQueued {
				queuedCount++
			}
		}
		res.Result = map[string]interface{}{
			"downloadSpeed":   "0",
			"uploadSpeed":     "0",
			"numActive":       activeCount,
			"numWaiting":      queuedCount,
			"numStopped":      len(tasks) - activeCount - queuedCount,
			"numStoppedTotal": len(tasks),
		}

	case "aria2.addUri":
		if len(req.Params) == 0 {
			res.Error = map[string]interface{}{"code": -32602, "message": "Invalid params"}
			return res
		}

		var rawUrls []string
		paramIdx := 0
		if len(req.Params) > 1 {
			if strVal, ok := req.Params[0].(string); ok && strings.HasPrefix(strVal, "token:") {
				paramIdx = 1
			}
		}

		if paramIdx < len(req.Params) {
			if urlList, ok := req.Params[paramIdx].([]interface{}); ok {
				for _, u := range urlList {
					if strUrl, ok := u.(string); ok && strUrl != "" {
						rawUrls = append(rawUrls, strUrl)
					}
				}
			}
		}

		if len(rawUrls) == 0 {
			res.Error = map[string]interface{}{"code": -32602, "message": "No URLs provided"}
			return res
		}

		// Enqueue into GlobalQueue
		format := "best"
		var firstTaskId string
		for _, u := range rawUrls {
			task := GlobalQueue.AddFull(u, format, "", "")
			if firstTaskId == "" && task != nil {
				firstTaskId = task.ID
			}
			log.Info().Msgf("Added task from Aria2 JSON-RPC: %s (%s)", task.ID, u)
		}

		if firstTaskId != "" {
			res.Result = firstTaskId
		} else {
			res.Result = "ok"
		}

	case "aria2.tellStatus":
		if len(req.Params) == 0 {
			res.Error = map[string]interface{}{"code": -32602, "message": "Missing GID"}
			return res
		}
		gid, _ := req.Params[0].(string)
		tasks := GlobalQueue.GetTasks()
		for _, t := range tasks {
			if t.ID == gid {
				statusStr := "waiting"
				if t.Status == StatusDownloading {
					statusStr = "active"
				} else if t.Status == StatusCompleted {
					statusStr = "complete"
				} else if t.Status == StatusFailed || t.Status == StatusCancelled {
					statusStr = "error"
				}
				res.Result = map[string]interface{}{
					"gid":             t.ID,
					"status":          statusStr,
					"totalLength":     t.TotalBytes,
					"completedLength": t.Downloaded,
					"downloadSpeed":   t.Speed,
					"errorMessage":    t.ErrorMessage,
				}
				return res
			}
		}
		res.Result = map[string]interface{}{"gid": gid, "status": "removed"}

	case "aria2.remove", "aria2.forceRemove":
		if len(req.Params) > 0 {
			if gid, ok := req.Params[0].(string); ok {
				GlobalQueue.Cancel(gid)
				res.Result = gid
				return res
			}
		}
		res.Result = "ok"

	default:
		res.Result = "ok"
	}

	return res
}
