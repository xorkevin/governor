source ./source.sh
out=defs
rm -rf $out
mkdir -p $out

docker-compose -f dc.main.yaml -f dc.prod.yaml -f dc.service.yaml config > $out/dc.gov.yaml
