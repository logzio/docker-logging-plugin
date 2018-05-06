FROM python:2
WORKDIR /usr/local/bin
COPY my_script.py .
ARG iterations
ENV ITERATIONS=$iterations
ARG prefix
ENV PREFIX=$prefix
ARG time
ENV TIME=$time

CMD python ./my_script.py -i $ITERATIONS -t $TIME -p $PREFIX