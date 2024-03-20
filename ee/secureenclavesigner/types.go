package secureenclavesigner

type SignResponseInner struct {
	Nonce     string `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
	Data      []byte `json:"data"`
}

type SignResponseOuter struct {
	Sig []byte `json:"sig"`
	Msg []byte `json:"msg"`
}
