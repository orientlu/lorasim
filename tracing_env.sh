#!/bin/bash
# by orientlu
export JAEGER_SAMPLER_TYPE="probabilistic"
#export JAEGER_SAMPLER_TYPE=const
export JAEGER_SAMPLER_PARAM=0.001
#export JAEGER_SAMPLER_PARAM=1
export JAEGER_REPORTER_LOG_SPANS=true
export JAEGER_AGENT_HOST="localhost"
export JAEGER_AGENT_PORT=6831

