package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/hertz-contrib/websocket"
	webrtc "github.com/pion/webrtc/v3"
)

var config = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
	},
}

var upgrader = websocket.HertzUpgrader{
	CheckOrigin: func(ctx *app.RequestContext) bool { return true },
}

func handleWebSocket(ctx context.Context, c *app.RequestContext) {
	err := upgrader.Upgrade(c, func(conn *websocket.Conn) {
		defer conn.Close()

		// 创建PeerConnection
		m := &webrtc.MediaEngine{}

		// 注册 PCMU 编解码器
		_ = m.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypePCMU,
				ClockRate: 8000,
				Channels:  1,
			},
			PayloadType: 0, // 0 = PCMU
		}, webrtc.RTPCodecTypeAudio)
		api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

		pc, err := api.NewPeerConnection(config)
		if err != nil {
			log.Println("创建PeerConnection失败:", err)
			return
		}
		defer pc.Close()

		// 本地回传Track（Opus）
		localTrack, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU},
			"loopback-audio",
			"loopback-stream",
		)
		if err != nil {
			log.Println("创建本地Track失败:", err)
			return
		}
		_, _ = pc.AddTrack(localTrack)

		// 收到远端Track
		pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
			log.Printf("收到远端轨道: %s, 类型: %s\n", track.ID(), track.Kind())
			go func() {
				for {
					pkt, _, err := track.ReadRTP()
					if err != nil {
						log.Println("ReadRTP错误:", err)
						return
					}
					fmt.Println("收到PCMU RTP包, 负载长度: ", len(pkt.Payload))
					// 写入本地Track，实现回环
					if err := localTrack.WriteRTP(pkt); err != nil {
						log.Println("WriteRTP错误:", err)
						return
					}
				}
			}()
		})

		// ICE候选者回调
		pc.OnICECandidate(func(cand *webrtc.ICECandidate) {
			if cand == nil {
				return
			}
			msg := map[string]interface{}{
				"type":      "candidate",
				"candidate": cand.ToJSON(),
			}
			b, _ := json.Marshal(msg)
			_ = conn.WriteMessage(websocket.TextMessage, b)
		})

		pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			log.Println("ICE状态变化:", state.String())
		})

		// WebSocket消息循环
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("WebSocket读取错误:", err)
				break
			}

			var msg map[string]interface{}
			_ = json.Unmarshal(message, &msg)
			fmt.Println("msg:", msg)

			switch msg["type"] {
			case "offer":
				offerBytes, _ := json.Marshal(msg)
				offer := webrtc.SessionDescription{}
				_ = json.Unmarshal(offerBytes, &offer)

				_ = pc.SetRemoteDescription(offer)
				answer, _ := pc.CreateAnswer(nil)
				gatherComplete := webrtc.GatheringCompletePromise(pc)
				_ = pc.SetLocalDescription(answer)
				<-gatherComplete

				b, _ := json.Marshal(pc.LocalDescription())
				_ = conn.WriteMessage(websocket.TextMessage, b)

			case "candidate":
				if msg["candidate"] != nil {
					fmt.Println("candidate:", msg)
					candBytes, _ := json.Marshal(msg["candidate"])
					candidate := webrtc.ICECandidateInit{}
					_ = json.Unmarshal(candBytes, &candidate)
					_ = pc.AddICECandidate(candidate)
					conn.Close()
				}
			}
		}
	})

	if err != nil {
		log.Println("WebSocket升级失败:", err)
	}
}

func main() {
	h := server.Default(server.WithHostPorts("0.0.0.0:3000"))

	h.Static("/", "./public")
	h.GET("/ws", handleWebSocket)

	log.Println("服务器启动: http://localhost:3000")
	h.Spin()
}
