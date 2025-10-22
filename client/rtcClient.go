package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"code.byted.org/aweme-go/hstruct/cast"
	"github.com/pion/webrtc/v3"
)

func main() {
	// 1. 初始化 WebRTC 配置
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create peer connection: %v", err)
	}

	// 2. 处理 ICE 候选者
	var candidates []*webrtc.ICECandidate

	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			// ICE 候选者收集完成后
			log.Println("ICE candidate collection complete.")
			return
		}
		// 打印 ICE 候选者信息，实际应用中应该发送给远端
		log.Printf("ICE Candidate: %s", candidate.ToJSON().Candidate)
		// 收集候选者并存储
		candidates = append(candidates, candidate)
		// 你可以通过 WebSocket 或 HTTP 请求将候选者发送给远端
	})

	// 3. 创建并发送 SDP offer
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		log.Fatalf("Failed to create offer: %v", err)
	}

	// 设置本地描述
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		log.Fatalf("Failed to set local description: %v", err)
	}

	// 将 Offer 发送到远程服务器
	offerJSON, err := json.Marshal(map[string]string{
		"type": cast.ToString(offer.Type),
		"sdp":  offer.SDP,
	})
	fmt.Println(string(offerJSON))

	if err != nil {
		log.Fatalf("Failed to marshal offer: %v", err)
	}

	url := "https://dcarx.dcarlife.net/motor/llm/agent_phone/dcc_phone_rtc"
	method := "POST"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, bytes.NewReader(offerJSON))

	if err != nil {
		fmt.Println(err)
		return
	}
	req.Header.Add("x-tt-env", "ppe_feat_newcar_tool")
	req.Header.Add("x-use-ppe", "1")
	req.Header.Add("User-Agent", "Apifox/1.0.0 (https://apifox.com)")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Host", "dcarx.dcarlife.net")
	req.Header.Add("Connection", "keep-alive")

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(body))

	// 4. 获取远程的 SDP answer
	var answer struct {
		Type string `json:"type"`
		Sdp  string `json:"sdp"`
	}

	err = json.Unmarshal(body, &answer)
	if err != nil {
		log.Fatalf("Failed to decode answer: %v", err)
	}

	// 5. 设置远程描述之前，先确保远端已收到 ICE 候选者
	// 在交换 SDP 完成后，你需要接收远端的 ICE 候选者
	for _, candidate := range candidates {
		log.Printf("Sending ICE candidate to remote: %s", candidate.ToJSON().Candidate)
		// 通过信令服务器或其他方法将候选者发送给远端
	}

	// 6. 设置远程描述
	err = peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: offer.Type,
		SDP:  answer.Sdp,
	})
	if err != nil {
		log.Fatalf("Failed to set remote description: %v", err)
	}

	// 7. 等待连接状态改变
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Connection State has changed: %s\n", state.String())
	})

	// 8. 保持程序运行
	select {}
}
