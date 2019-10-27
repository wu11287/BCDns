package service

import (
	"BCDns_0.1/bcDns/conf"
	"BCDns_0.1/certificateAuthority/service"
	"BCDns_0.1/messages"
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
)

const (
	DeadType = iota
	TranMiss
	BlockOvertime
)

const (
	Start = iota
	Ready
	OnChange
)

var (
	Leader LeaderT
)

type LeaderT struct {
	OnChanging        sync.Mutex
	LeaderId          int64
	TermId            int64
	RetrieveMsgs      map[int64]map[string]int64
	RetrieveMsgsCount map[int64]map[int64]int
	ViewChangeMsgs    map[int64]map[string]uint8
	State             uint8
}

func (leader *LeaderT) ProcessViewChangeMsg() {
	var msg ViewChangeMsg
	for {
		select {
		case msgByte := <-ViewChangeMsgChan:
			err := json.Unmarshal(msgByte, msg)
			if err != nil {
				fmt.Println("Process viewchange msg failed", err)
				continue
			}
			if msg.TermId != leader.TermId {
				fmt.Println("Outdated msg")
				continue
			}
			if !checkType(msg.ViewChangeType) {
				fmt.Println("Illegal msg type")
				continue
			}
			if !service.CertificateAuthorityX509.Exits(msg.From) {
				fmt.Printf("[ProcessViewChangeMsg] unexist node name=%v\n", msg.From)
				continue
			}
			if !msg.VerifySignature() {
				fmt.Printf("[ProcessViewChangeMsg] Invalid signature\n")
				continue
			}
			if _, ok := leader.ViewChangeMsgs[msg.TermId]; ok {
				if _, ok := leader.ViewChangeMsgs[msg.TermId][msg.From]; !ok {
					leader.ViewChangeMsgs[msg.TermId][msg.From] = 0
					if len(leader.ViewChangeMsgs[msg.TermId]) >= service.CertificateAuthorityX509.GetF()*2+1 {

					}
				}
			}
		case msgByte := <-ViewChangeResultChan:

		}
	}
}

func (leader *LeaderT) ChangeLeader() {

}

func (leader *LeaderT) ProcessRetrieveMsg() {
	for {
		select {
		case msgByte := <-RetrieveLeaderMsgChan:
			var msg ViewRetrieveMsg
			err := json.Unmarshal(msgByte, msg)
			if err != nil {
				fmt.Println("[ProcessRetrieveMsg] json.Unmarshal msg failed", err)
				continue
			}
			leader.OnChanging.Lock()
			defer leader.OnChanging.Unlock()
			response := ViewInfo{
				TermId:   leader.TermId,
				LeaderId: leader.LeaderId,
				From:     conf.BCDnsConfig.HostName,
			}
			response.Signature, err = response.Sign()
			if err != nil {
				fmt.Printf("[ProcessRetrieveMsg] err=%v\n", err)
				continue
			}
			responseByte, err := json.Marshal(msg)
			if err != nil {
				fmt.Printf("[ProcessRetrieveMsg] json.Marshal failed err=%v\n", err)
				continue
			}
			P2PNet.SendTo(responseByte, RetrieveLeaderResponse, msg.From)
		case msgByte := <-RetrieveLeaderResponseChan:
			var msg ViewInfo
			err := json.Unmarshal(msgByte, &msg)
			if err != nil {
				fmt.Printf("[ProcessRetrieveMsg] json.Unmarshal msg failed err=%v\n", err)
				continue
			}
			if msg.LeaderId < leader.TermId {
				fmt.Printf("[ViewRetrieveResponse] Mgs's leaderId is too small\n")
				continue
			}
			if !msg.VerifySignature() {
				fmt.Printf("[ViewRetrieveResponse] invalid sig\n")
				continue
			}
			//TODO: Store message by lru
			if _, ok := leader.RetrieveMsgs[msg.TermId]; ok {
				leader.RetrieveMsgs[msg.TermId][msg.From] = msg.LeaderId
				if _, ok := leader.RetrieveMsgs[msg.TermId][msg.From]; !ok {
					leader.RetrieveMsgs[msg.TermId][msg.From] = msg.LeaderId
					if _, ok := leader.RetrieveMsgsCount[msg.TermId][msg.LeaderId]; ok {
						leader.RetrieveMsgsCount[msg.TermId][msg.LeaderId]++
					} else {
						leader.RetrieveMsgsCount[msg.TermId][msg.LeaderId] = 1
					}
					if leader.RetrieveMsgsCount[msg.TermId][msg.LeaderId] >= 2*service.CertificateAuthorityX509.GetF()+1 {
						leader.TermId = msg.TermId
						leader.LeaderId = msg.LeaderId
						leader.State = Ready
					}
				}
			}
		}
	}
}

func (leader *LeaderT) Run() {

}

type ViewChangeMsg struct {
	ViewChangeMsgData
	Signature []byte
}

func (m ViewChangeMsg) Hash() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(m.ViewChangeMsgData); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m ViewChangeMsg) Sign() ([]byte, error) {
	hash, err := m.Hash()
	if err != nil {
		return nil, err
	}
	if sig := service.CertificateAuthorityX509.Sign(hash); sig != nil {
		return sig, nil
	}
	return nil, errors.New("Generate signature failed")
}

func (m ViewChangeMsg) VerifySignature() bool {
	hash, err := m.Hash()
	if err != nil {
		fmt.Printf("[VerifySignature] ViewRetrieveResponse's signature is illegle err=%v\n", err)
		return false
	}
	if service.CertificateAuthorityX509.VerifySignature(m.Signature, hash, m.From) {
		return true
	}
	return false
}

//type ViewChangeResult struct {
//	TermId, LeaderId int64
//	From             string
//	Signature        []byte
//}

type ViewChangeMsgData struct {
	Type             uint8
	From             string
	ViewChangeType   int
	TermId, LeaderId int64
	//key is PId'String
	TId messages.PId
}

//type LeaderVoteMsg struct {
//	Type uint8
//	Msgs []ViewChangeMsg
//}

type LeaderTInterface interface {
	ProcessViewChangeMsg()
	LeaderVote(ViewChangeMsgData)
	ProcessRetrieveMsg()
	Run()
}

type ViewRetrieveMsg struct {
	From string
}

type ViewInfo struct {
	TermId, LeaderId int64
	From             string
	Signature        []byte
}

func (r ViewInfo) Hash() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(r.TermId); err != nil {
		return nil, err
	}
	if err := enc.Encode(r.LeaderId); err != nil {
		return nil, err
	}
	if err := enc.Encode(r.From); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil

}

func (r ViewInfo) Sign() ([]byte, error) {
	hash, err := r.Hash()
	if err != nil {
		return nil, err
	}
	if sig := service.CertificateAuthorityX509.Sign(hash); sig != nil {
		return sig, nil
	}
	return nil, errors.New("Generate signature failed")
}

func (r ViewInfo) VerifySignature() bool {
	hash, err := r.Hash()
	if err != nil {
		fmt.Printf("[VerifySignature] ViewRetrieveResponse's signature is illegle err=%v\n", err)
		return false
	}
	if service.CertificateAuthorityX509.VerifySignature(r.Signature, hash, r.From) {
		return true
	}
	return false
}

func init() {
	msg := ViewRetrieveMsg{}
	msgByte, err := json.Marshal(msg)
	if err != nil {
		log.Fatal("Leader init failed", err)
	}
	P2PNet.BroadcastMsg(msgByte, RetrieveLeader)
	Leader = LeaderT{
		LeaderId:          -1,
		TermId:            -1,
		RetrieveMsgs:      make(map[int64]map[string]int64, 0),
		RetrieveMsgsCount: make(map[int64]map[int64]int),
		ViewChangeMsgs:    make(map[int64]map[string]uint8, 0),
		State:             Start,
	}
}

//static method
func (leader *LeaderT) TurnLeader() {
	service.CertificateAuthorityX509.Mutex.Lock()
	defer service.CertificateAuthorityX509.Mutex.Unlock()

	leader.OnChanging.Lock()
	defer leader.OnChanging.Unlock()

	Leader.LeaderId = (Leader.LeaderId + 1) % int64(service.CertificateAuthorityX509.GetNetworkSize())
}

func checkType(t int) bool {
	if t >= DeadType && t <= BlockOvertime {
		return true
	}
	return false
}
