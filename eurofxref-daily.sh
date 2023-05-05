rm -f eurofxref-daily.csv
rm -f eurofxref-daily.xml

curl --silent https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml > eurofxref-daily.xml

usdeur=$(grep -Pio 'currency=\x27USD\x27.*rate=\x27\K[^\x27]*' eurofxref-daily.xml)
zareur=$(grep -Pio 'currency=\x27ZAR\x27.*rate=\x27\K[^\x27]*' eurofxref-daily.xml)

echo $usdeur,$zareur > eurofxref-daily.csv
