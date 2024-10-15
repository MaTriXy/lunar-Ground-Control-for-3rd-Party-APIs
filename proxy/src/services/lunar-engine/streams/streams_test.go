package streams

import (
	"errors"
	"lunar/engine/messages"
	streamconfig "lunar/engine/streams/config"
	testprocessors "lunar/engine/streams/flow/test-processors"
	internaltypes "lunar/engine/streams/internal-types"
	"lunar/engine/streams/processors"
	filterprocessor "lunar/engine/streams/processors/filter-processor"
	publictypes "lunar/engine/streams/public-types"
	streamtypes "lunar/engine/streams/types"
	"lunar/engine/utils/environment"
	contextmanager "lunar/toolkit-core/context-manager"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	prevVal := environment.SetProcessorsDirectory(filepath.Join("flow", "test-processors"))

	contextmanager.Get().SetMockClock()

	// Run the tests
	code := m.Run()

	// Clean up if necessary
	environment.SetProcessorsDirectory(prevVal)

	// Exit with the code from the tests
	os.Exit(code)
}

func TestNewStream(t *testing.T) {
	stream, err := NewStream()
	require.NoError(t, err, "Failed to create stream")
	require.NotNil(t, stream, "stream is nil")
	require.NotNil(t, stream.apiStreams, "APIStreams is nil")
	require.NotNil(t, stream.filterTree, "filterTree is nil")
}

func TestExecuteFlows(t *testing.T) {
	procMng := createTestProcessorManager(t, []string{"removePII", "readCache", "checkLimit", "generateResponse", "globalStream", "writeCache", "LogAPM", "readXXX", "writeXXX"})
	stream, err := NewStream()
	require.NoError(t, err, "Failed to create stream")
	stream.processorsManager = procMng

	flowReps := createFlowRepresentation(t, "2-flows*")
	err = stream.createFlows(flowReps)
	require.NoError(t, err, "Failed to create flows")

	apiStream := streamtypes.NewAPIStream("APIStreamName", publictypes.StreamTypeRequest)
	apiStream.SetRequest(streamtypes.NewRequest(messages.OnRequest{
		Method:  "GET",
		Scheme:  "https",
		URL:     "maps.googleapis.com/maps/api/geocode/json",
		Headers: map[string]string{},
	}))
	apiStream.SetResponse(streamtypes.NewResponse(messages.OnResponse{
		Status: 200,
	}))

	flowActions := &streamconfig.StreamActions{
		Request:  &streamconfig.RequestStream{},
		Response: &streamconfig.ResponseStream{},
	}
	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")

	apiStream.SetType(publictypes.StreamTypeResponse)
	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")

	// Test for 3 flows

	stream, err = NewStream()
	require.NoError(t, err, "Failed to create stream")
	stream.processorsManager = procMng

	flowReps = createFlowRepresentation(t, "3-flows*")
	err = stream.createFlows(flowReps)
	require.NoError(t, err, "Failed to create flows")

	apiStream = streamtypes.NewAPIStream("APIStreamName", publictypes.StreamTypeRequest)
	apiStream.SetRequest(streamtypes.NewRequest(messages.OnRequest{
		Method:  "GET",
		Scheme:  "https",
		URL:     "www.whatever.com/blabla",
		Headers: map[string]string{},
	}))
	apiStream.SetResponse(streamtypes.NewResponse(messages.OnResponse{
		Status: 200,
	}))

	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")
}

func TestCreateFlows(t *testing.T) {
	procMng := createTestProcessorManager(t, []string{"removePII", "readCache", "checkLimit", "generateResponse", "globalStream", "writeCache", "LogAPM", "readXXX", "writeXXX"})
	stream, err := NewStream()
	require.NoError(t, err, "Failed to create stream")

	stream.processorsManager = procMng
	flowReps := createFlowRepresentation(t, "2-flows*")

	err = stream.createFlows(flowReps)
	require.NoError(t, err, "Failed to create flows")

	stream, err = NewStream()
	require.NoError(t, err, "Failed to create stream")

	stream.processorsManager = procMng
	flowReps = createFlowRepresentation(t, "3-flows*")
	err = stream.createFlows(flowReps)
	require.NoError(t, err, "Failed to create flows")
}

func TestEarlyResponseFlow(t *testing.T) {
	procMng := createTestProcessorManagerWithFactories(t, []string{"readCache", "writeCache", "generateResponse", "LogAPM"},
		testprocessors.NewMockProcessorUsingCache,
		testprocessors.NewMockProcessor,
		testprocessors.NewMockGenerateResponseProcessor,
		testprocessors.NewMockProcessor,
	)
	stream, err := NewStream()
	require.NoError(t, err, "Failed to create stream")
	stream.processorsManager = procMng

	flowReps := createFlowRepresentation(t, "early-response-test-case")
	err = stream.createFlows(flowReps)
	require.NoError(t, err, "Failed to create flows")

	contextManager := streamtypes.NewContextManager()
	globalContext := contextManager.GetGlobalContext()

	apiStream := streamtypes.NewAPIStream("APIStreamName", publictypes.StreamTypeRequest)
	apiStream.SetRequest(streamtypes.NewRequest(messages.OnRequest{
		Method:  "GET",
		Scheme:  "https",
		URL:     "maps.googleapis.com/maps/api/geocode/json",
		Headers: map[string]string{},
	}))
	apiStream.SetResponse(streamtypes.NewResponse(messages.OnResponse{
		Status: 200,
		URL:    "maps.googleapis.com/maps/api/geocode/json",
	}))
	flowActions := &streamconfig.StreamActions{
		Request:  &streamconfig.RequestStream{},
		Response: &streamconfig.ResponseStream{},
	}

	// simulate early response
	err = globalContext.Set(testprocessors.GlobalKeyExecutionOrder, []string{})
	require.NoError(t, err, "Failed to set global context value")
	err = globalContext.Set(testprocessors.GlobalKeyCacheHit, true)
	require.NoError(t, err, "Failed to set global context value")

	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")

	execOrder, err := globalContext.Get(testprocessors.GlobalKeyExecutionOrder)
	require.NoError(t, err, "Failed to get global context value")
	require.Equal(t, []string{"readCache", "generateResponse", "LogAPM"}, execOrder, "Execution order is not correct")

	// simulate regular execution
	err = globalContext.Set(testprocessors.GlobalKeyCacheHit, false)
	require.NoError(t, err, "Failed to set global context value")
	err = globalContext.Set(testprocessors.GlobalKeyExecutionOrder, []string{})
	require.NoError(t, err, "Failed to set global context value")

	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")

	execOrder, err = globalContext.Get(testprocessors.GlobalKeyExecutionOrder)
	require.NoError(t, err, "Failed to get global context value")
	require.Equal(t, []string{"readCache"}, execOrder, "Execution order is not correct")

	// simulate API provider response
	apiStream.SetType(publictypes.StreamTypeResponse)
	err = globalContext.Set(testprocessors.GlobalKeyExecutionOrder, []string{})
	require.NoError(t, err, "Failed to set global context value")

	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")

	execOrder, err = globalContext.Get(testprocessors.GlobalKeyExecutionOrder)
	require.NoError(t, err, "Failed to get global context value")
	require.Equal(t, []string{"writeCache"}, execOrder, "Execution order is not correct")
}

func TestLunarGlobalContextUsage(t *testing.T) {
	procMng := createTestProcessorManagerWithFactories(t, []string{"processor1", "processor2"},
		testprocessors.NewMockProcessorUsingGlobalContextSrc,
		testprocessors.NewMockProcessorUsingGlobalContextDest,
	)

	contextManager := streamtypes.NewContextManager()
	globalContext := contextManager.GetGlobalContext()
	err := globalContext.Set(testprocessors.GlobalKey, testprocessors.GlobalValue)
	require.NoError(t, err, "Failed to set global context value")

	stream := createStreamForContextTest(t, procMng)
	apiStream := createAPIStreamForContextTest()

	executionContext := getExecutionContext(stream, apiStream)

	require.Equal(t, globalContext, executionContext.GetGlobalContext(), "Global context is not the same")

	err = executionContext.GetGlobalContext().Set(testprocessors.GlobalKey, testprocessors.GlobalValue)
	require.NoError(t, err, "Failed to set global context value")

	runContextTest(t, stream, apiStream)

	// Check if the global context has been used
	outVal, err := contextManager.GetGlobalContext().Get(testprocessors.GlobalKey)
	require.NoError(t, err, "Failed to get global context value")
	require.Equal(t, testprocessors.UsedValue, outVal, "Global context is not used")

	executionContext = getExecutionContext(stream, apiStream)
	require.Equal(t, globalContext, executionContext.GetGlobalContext(), "Global context is not the same")
}

func TestLunarFlowContextUsage(t *testing.T) {
	procMng := createTestProcessorManagerWithFactories(t, []string{"processor1", "processor2"},
		testprocessors.NewMockProcessorUsingFlowContextSrc,
		testprocessors.NewMockProcessorUsingFlowContextDest,
	)

	stream := createStreamForContextTest(t, procMng)
	apiStream := createAPIStreamForContextTest()
	runContextTest(t, stream, apiStream)

	// Check if the flow context has been used
	outVal, err := getExecutionContext(stream, apiStream).GetFlowContext().Get(testprocessors.FlowKey)
	require.NoError(t, err, "Failed to get flow context value")
	require.Equal(t, testprocessors.UsedValue, outVal, "Flow context is not used")
}

func TestLunarTransactionalContextUsage(t *testing.T) {
	procMng := createTestProcessorManagerWithFactories(t, []string{"processor1", "processor2"},
		testprocessors.NewMockProcessorUsingTrContextSrc,
		testprocessors.NewMockProcessorUsingTrContextDest,
	)

	stream := createStreamForContextTest(t, procMng)
	apiStream := createAPIStreamForContextTest()
	runContextTest(t, stream, apiStream)

	ctx := getExecutionContext(stream, apiStream)

	// Check that the transactional context was removed
	require.Nil(t, ctx.GetTransactionalContext(), "Transactional context is not removed")

	// Check if the transaction context has been used
	outVal, err := ctx.GetGlobalContext().Get(testprocessors.TransactionalKey)
	require.NoError(t, err, "Failed to get context value")
	require.Equal(t, testprocessors.UsedValue, outVal, "Transactional context is not used")
}

func TestFilterProcessorFlow(t *testing.T) {
	procMng := createTestProcessorManagerWithFactories(t, []string{"Filter", "generateResponse", "LogAPM"},
		filterprocessor.NewProcessor,
		testprocessors.NewMockGenerateResponseProcessor,
		testprocessors.NewMockProcessor,
	)

	stream, err := NewStream()
	require.NoError(t, err, "Failed to create stream")
	stream.processorsManager = procMng

	flowReps := createFlowRepresentation(t, "filter*")
	err = stream.createFlows(flowReps)
	require.NoError(t, err, "Failed to create flows")

	contextManager := streamtypes.NewContextManager()
	globalContext := contextManager.GetGlobalContext()

	apiStream := streamtypes.NewAPIStream("APIStreamName", publictypes.StreamTypeRequest)
	request := messages.OnRequest{
		Method:  "GET",
		Scheme:  "https",
		URL:     "maps.googleapis.com/maps/api/geocode/json",
		Headers: map[string]string{"X-Group": "production"},
	}
	apiStream.SetRequest(streamtypes.NewRequest(request))

	// execution for production environment
	err = globalContext.Set(testprocessors.GlobalKeyExecutionOrder, []string{})
	require.NoError(t, err, "Failed to set global context value")

	flowActions := &streamconfig.StreamActions{
		Request:  &streamconfig.RequestStream{},
		Response: &streamconfig.ResponseStream{},
	}

	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")

	execOrder, err := globalContext.Get(testprocessors.GlobalKeyExecutionOrder)
	require.NoError(t, err, "Failed to get global context value")
	require.Equal(t, []string{"LogAPM"}, execOrder, "Execution order is not correct")

	// execution for staging environment
	request.Headers["X-Group"] = "staging"
	apiStream.SetRequest(streamtypes.NewRequest(request))

	err = globalContext.Set(testprocessors.GlobalKeyExecutionOrder, []string{})
	require.NoError(t, err, "Failed to set global context value")

	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")

	execOrder, err = globalContext.Get(testprocessors.GlobalKeyExecutionOrder)
	require.NoError(t, err, "Failed to get global context value")
	require.Equal(t, []string{"GenerateResponseTooManyRequests"}, execOrder, "Execution order is not correct")

	// execution for development environment
	request.Headers["X-Group"] = "development"
	apiStream.SetRequest(streamtypes.NewRequest(request))

	err = globalContext.Set(testprocessors.GlobalKeyExecutionOrder, []string{})
	require.NoError(t, err, "Failed to set global context value")

	err = stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")

	execOrder, err = globalContext.Get(testprocessors.GlobalKeyExecutionOrder)
	require.NoError(t, err, "Failed to get global context value")

	require.Equal(t, []string{"LogAPM"}, execOrder, "Execution order is not correct")
}

func TestMeasureFlowExecutionTime(t *testing.T) {
	metrics := newFlowMetricsData()

	for i := 0; i < 5; i++ {
		err := metrics.measureFlowExecutionTime(func() error {
			time.Sleep(50 * time.Millisecond) // Simulate some work
			return nil
		})
		require.NoError(t, err)
	}

	avgTime := metrics.getAvgFlowExecutionTime()
	require.Greater(t, avgTime, 0.0)

	// Ensure that the average is within the expected range
	require.InDelta(t, 50, avgTime, 10) // Allowing some delta (±10ms) due to possible variations in execution time

	// Call the measured function with an error to check that it doesn't affect the average
	err := metrics.measureFlowExecutionTime(func() error {
		return errors.New("test error")
	})
	require.Error(t, err)
	require.Equal(t, "test error", err.Error())

	// average execution time should be still the same after the error
	avgTimeAfterError := metrics.getAvgFlowExecutionTime()
	require.Equal(t, avgTime, avgTimeAfterError)
}

func createTestProcessorManager(t *testing.T, processorNames []string) *processors.ProcessorManager {
	return createTestProcessorManagerWithFactories(t, processorNames, testprocessors.NewMockProcessor)
}

func createTestProcessorManagerWithFactories(t *testing.T, processorNames []string, factories ...processors.ProcessorFactory) *processors.ProcessorManager {
	processorMng := processors.NewProcessorManager(nil)
	for i, procName := range processorNames {
		factory := factories[0]
		if len(factories) > 1 {
			factory = factories[i]
		}
		processorMng.SetFactory(procName, factory)
	}

	err := processorMng.Init()
	require.NoError(t, err)

	return processorMng
}

func createFlowRepresentation(t *testing.T, testCase string) map[string]internaltypes.FlowRepI {
	pattern := filepath.Join("flow", "test-cases", testCase, "*.yaml")
	files, fileErr := filepath.Glob(pattern)
	require.NoError(t, fileErr, "Failed to find YAML files")

	flowReps := make(map[string]internaltypes.FlowRepI)
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			flowRep, err := streamconfig.ReadStreamFlowConfig(file)
			require.NoError(t, err, "Failed to read YAML file")

			// TODO: We should do it more strict that we load the processor keys
			for key, proc := range flowRep.Processors {
				proc.Key = key
				flowRep.Processors[key] = proc
			}
			flowReps[flowRep.Name] = flowRep
		})
	}
	return flowReps
}

func createStreamForContextTest(t *testing.T, procMng *processors.ProcessorManager) *Stream {
	stream, err := NewStream()
	require.NoError(t, err, "Failed to create stream")
	stream.processorsManager = procMng

	globalStreamRefStart := &streamconfig.StreamRef{Name: publictypes.GlobalStream, At: "start"}
	globalStreamRefEnd := &streamconfig.StreamRef{Name: publictypes.GlobalStream, At: "end"}
	processorRef1 := &streamconfig.ProcessorRef{Name: "processor1"}
	processorRef2 := &streamconfig.ProcessorRef{Name: "processor2"}
	flowReps := map[string]internaltypes.FlowRepI{
		"GraphWithEntryPoints": &streamconfig.FlowRepresentation{
			Filter: &streamconfig.Filter{URL: "maps.googleapis.com/*"},
			Name:   "GraphWithEntryPoints",
			Processors: map[string]*streamconfig.Processor{
				"processor1": {Processor: "processor1", Key: "processor1"},
				"processor2": {Processor: "processor2", Key: "processor2"},
			},
			Flow: streamconfig.Flow{
				Request: []*streamconfig.FlowConnection{
					{
						From: &streamconfig.Connection{Stream: globalStreamRefStart},
						To:   &streamconfig.Connection{Processor: processorRef1},
					},
					{
						From: &streamconfig.Connection{Processor: processorRef1},
						To:   &streamconfig.Connection{Processor: processorRef2},
					},
					{
						From: &streamconfig.Connection{Processor: processorRef2},
						To:   &streamconfig.Connection{Stream: globalStreamRefEnd},
					},
				},
			},
		},
	}
	err = stream.createFlows(flowReps)
	require.NoError(t, err, "Failed to create flows")

	return stream
}

func createAPIStreamForContextTest() publictypes.APIStreamI {
	apiStream := streamtypes.NewAPIStream("APIStreamName", publictypes.StreamTypeRequest)
	apiStream.SetRequest(streamtypes.NewRequest(messages.OnRequest{
		Method:  "GET",
		Scheme:  "https",
		URL:     "maps.googleapis.com/maps/api/geocode/json",
		Headers: map[string]string{},
	}))
	apiStream.SetResponse(streamtypes.NewResponse(messages.OnResponse{
		Status: 200,
	}))

	return apiStream
}

func runContextTest(t *testing.T, stream *Stream, apiStream publictypes.APIStreamI) {
	flowActions := &streamconfig.StreamActions{
		Request:  &streamconfig.RequestStream{},
		Response: &streamconfig.ResponseStream{},
	}
	err := stream.ExecuteFlow(apiStream, flowActions)
	require.NoError(t, err, "Failed to execute flow")
}

func getExecutionContext(stream *Stream, apiStream publictypes.APIStreamI) publictypes.LunarContextI {
	return stream.filterTree.GetFlow(apiStream)[0].GetExecutionContext()
}
