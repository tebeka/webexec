package webexec

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/pion/webrtc/v2"
)

func signalPair(pcOffer *webrtc.PeerConnection, pcAnswer *webrtc.PeerConnection) error {
	iceGatheringState := pcOffer.ICEGatheringState()
	offerChan := make(chan webrtc.SessionDescription, 1)

	if iceGatheringState != webrtc.ICEGatheringStateComplete {
		pcOffer.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate == nil {
				offerChan <- *pcOffer.PendingLocalDescription()
			}
		})
	}
	// Note(albrow): We need to create a data channel in order to trigger ICE
	// candidate gathering in the background for the JavaScript/Wasm bindings. If
	// we don't do this, the complete offer including ICE candidates will never be
	// generated.
	if _, err := pcOffer.CreateDataChannel("signaling", nil); err != nil {
		return err
	}

	offer, err := pcOffer.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pcOffer.SetLocalDescription(offer); err != nil {
		return err
	}

	if iceGatheringState == webrtc.ICEGatheringStateComplete {
		offerChan <- offer
	}
	select {
	case <-time.After(3 * time.Second):
		return fmt.Errorf("timed mockedMsgs waiting to receive offer")
	case offer := <-offerChan:
		if err := pcAnswer.SetRemoteDescription(offer); err != nil {
			return err
		}

		answer, err := pcAnswer.CreateAnswer(nil)
		if err != nil {
			return err
		}

		if err = pcAnswer.SetLocalDescription(answer); err != nil {
			return err
		}

		err = pcOffer.SetRemoteDescription(answer)
		if err != nil {
			return err
		}
		return nil
	}
}

var mockedMsgs []string

func mockSender(msg []byte) error {
	fmt.Printf("Mock sending - %v", msg)
	mockedMsgs = append(mockedMsgs, string(msg))
	return nil
}
func TestReadNSend(t *testing.T) {
	var err error
	cmd := exec.Command("echo", "hello", "world")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to open cmd %q: %v", cmd, err)
	}
	err = cmd.Start()
	if err != nil {
		t.Fatalf("Failed to run command %q", err)
	}
	err = ReadNSend(stdout, mockSender)
	if err != nil {
		t.Fatalf("ReanNSend return an err - %q", err)
	}
	if len(mockedMsgs) != 1 {
		t.Fatalf("Bad mockedMsgsput len %d", len(mockedMsgs))
	}
	if mockedMsgs[0] != "hello world\n" {
		t.Fatalf("Bad mockedMsgsput string %q", mockedMsgs[0])
	}

}
func TestSimpleEcho(t *testing.T) {
	done := make(chan bool)
	server, err := NewServer(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("echo hello world", nil)
	dc.OnOpen(func() {
		fmt.Println("Channel opened")
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if !msg.IsString && string(msg.Data) != "hello world\n" {
			t.Fatalf("Got bad msg: %q", msg.Data)
		}
	})
	dc.OnClose(func() {
		fmt.Println("Client Data channel closed")
		done <- true
	})
	signalPair(client, server)
	<-done
}
func TestMultiLine(t *testing.T) {
	done := make(chan bool)
	server, err := NewServer(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("Failed to start a new server %v", err)
	}
	dc, err := client.CreateDataChannel("cat <<EOF", nil)
	dc.OnOpen(func() {
		dc.Send([]byte("123\n"))
		dc.Send([]byte("456\n"))
		dc.Send([]byte("EOF\n"))
		fmt.Println("Finished sending")
	})
	var mockedMsgs []string
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		data := string(msg.Data)
		mockedMsgs = append(mockedMsgs, data)
	})
	dc.OnClose(func() {
		fmt.Println("On closing the data channel")
		done <- true

	})
	signalPair(client, server)
	<-done
	if len(mockedMsgs) != 2 {
		t.Fatalf("Wrong number of strings in mockedMsgs - %v", mockedMsgs)
	}
	if mockedMsgs[0] != "123" || mockedMsgs[1] != "456" {
		t.Fatalf("Got bad mockedMsgsput - %v", mockedMsgs)
	}
}
