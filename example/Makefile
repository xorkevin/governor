.PHONY: devinit devinit2 devup

devinit:
	./plan.sh
	./register.sh
	./connect.sh

devup:
	kubectl apply -k base

devinit2:
	./register2.sh
