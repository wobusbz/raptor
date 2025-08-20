package main

type Player struct {
	PlyID int64

	// Buff 系统
	// 属性系统
	// 背包系统
}

type PlayerManager struct {
	plys map[int64]*Player
}

type Session interface {
	Push(msg any)
	RPC(msg any)
}

type N2MLoginRequest struct {
	PlyID int64
}

var playerManager = &PlayerManager{plys: make(map[int64]*Player)}

func N2MLogin(session Session, data *N2MLoginRequest) {
	playerManager.plys[data.PlyID] = &Player{}
}

type C2SEnterGameRequest struct {
}

func C2SEnterGame(session Session, message C2SEnterGameRequest) {
	// 这里如何获取Player玩家
	// 如何把这个消息注册到这个服务器的整个框架里面去
	// 把这些都高度集成到框架里面
	// 这里如何调用模块里面的函数 ?
}
