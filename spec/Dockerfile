FROM gw-base:latest

WORKDIR /app

COPY legal/astan/legal-inquiry-v1-astan.yaml /app/config.yaml

#ENV APP_ENV=production
#ENV APP_ENV=production

ENV INQUIRY_USERNAME=melal_pgsb
ENV INQUIRY_PASSWORD=MeLL@L87@9

EXPOSE 8080

CMD ["-config", "/app/config.yaml"]
