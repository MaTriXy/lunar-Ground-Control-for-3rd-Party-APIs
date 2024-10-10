@mainTests
Feature: Lunar Proxy MetricsCollector Diagnosis With Flows
    Background: Starts the Proxy
        Given   API Provider is up
        # The next 2 steps are mandatory in order to clean OTEL state.
        # TODO use future `reset` functionality instead and save some time 💪
        Given   Lunar Proxy is down
        And     Lunar Proxy env var `LUNAR_STREAMS_ENABLED` set to `true`
        And     Lunar Proxy is up
   
    Scenario: Request is exported to Prometheus metric server
        When    Basic rate limit flow created for httpbinmock/* with 5 requests per 1 seconds
        And     flow file is saved
        And     resource file is saved

        And     load_flows command is run

        And     next epoch-based 1 seconds window arrives

        And     A request to httpbinmock /status/200 is made through Lunar Proxy
        And     A request to httpbinmock /status/200 is made through Lunar Proxy
        And     A request to httpbinmock /status/200 is made through Lunar Proxy        

        
        Then    There is a counter named api_call_count with the value 3
