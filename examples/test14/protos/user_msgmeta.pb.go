package protos

func (*C2SLogin) ID() uint32    { return 1 }
func (*C2SLogin) Route() string { return "cent.user.C2SLogin" }

func (*S2CLogin) ID() uint32    { return 2 }
func (*S2CLogin) Route() string { return "cent.user.S2CLogin" }
