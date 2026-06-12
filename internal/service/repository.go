package service

type Repository interface {
	CreateIp() (Ip.ip, error)
}
