FROM gcr.io/distroless/static-debian11:nonroot
ENTRYPOINT ["/baton-ramp"]
COPY baton-ramp /