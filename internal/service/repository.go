package service
import (
	
)
type Repository interface {
	CreateIp() (Ip.ip, error)
}
