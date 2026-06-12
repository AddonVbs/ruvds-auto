package internal

type Ip struct {
	ip_1 int `gorm:"columb:ipAdress_1;not null"`
	ip_2 int `gorm:"columb:ipAdress_2"`
	ip_3 int `gorm:"columb:ipAdress_3"`
	ip_4 int `gorm:"columb:ipAdress_4"`
	ip_5 int `gorm:"columb:ipAdress_5"`
	ip_6 int `gorm:"columb:ipAdress_6"`
}
