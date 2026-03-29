package solid

// FatInterface has too many methods — triggers ISP violation.
type FatInterface interface {
	MethodA()
	MethodB()
	MethodC()
	MethodD()
	MethodE()
	MethodF()
	MethodG()
	MethodH()
	MethodI()
}
