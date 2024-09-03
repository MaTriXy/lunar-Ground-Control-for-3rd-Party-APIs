package routing

import (
	"fmt"
	"lunar/engine/communication"
	"lunar/engine/config"
	"lunar/engine/runner"
	"lunar/engine/services"
	"lunar/engine/streams"
	"lunar/engine/utils/environment"
	"lunar/engine/utils/writers"
	contextmanager "lunar/toolkit-core/context-manager"
	"lunar/toolkit-core/otel"
	"net/http"
	"time"

	sharedConfig "lunar/shared-model/config"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
)

const (
	lunarEngine            string = "lunar-engine"
	syslogExporterEndpoint string = "127.0.0.1:5140"
)

type PoliciesData struct {
	diagnosisWorker   *runner.DiagnosisWorker
	configBuildResult config.BuildResult
}

type StreamsData struct {
	stream *streams.Stream
}

type HandlingDataManager struct {
	PoliciesData
	StreamsData

	isStreamsEnabled bool
	lunarHub         *communication.HubCommunication
	writer           writers.Writer
	proxyTimeout     time.Duration
	policiesServices *services.PoliciesServices

	shutdown              func()
	areMetricsInitialized bool
}

func NewHandlingDataManager(
	proxyTimeout time.Duration,
	hubComm *communication.HubCommunication,
) *HandlingDataManager {
	ctxMng := contextmanager.Get()
	data := &HandlingDataManager{
		proxyTimeout: proxyTimeout,
		lunarHub:     hubComm,
		writer:       writers.Dial("tcp", syslogExporterEndpoint, ctxMng.GetClock()),
	}
	return data
}

func (rd *HandlingDataManager) Setup() error {
	if environment.IsStreamsEnabled() {
		return rd.initializeStreams()
	}
	return rd.initializePolicies()
}

func (rd *HandlingDataManager) RunDiagnosisWorker() {
	if rd.diagnosisWorker == nil {
		return
	}
	rd.diagnosisWorker.Run(
		rd.configBuildResult.Accessor,
		&rd.policiesServices.Diagnosis,
		&rd.policiesServices.Exporters,
	)
}

func (rd *HandlingDataManager) StopDiagnosisWorker() {
	if rd.diagnosisWorker != nil {
		rd.diagnosisWorker.Stop()
	}
}

func (rd *HandlingDataManager) GetTxnPoliciesAccessor() *config.TxnPoliciesAccessor {
	return rd.configBuildResult.Accessor
}

func (rd *HandlingDataManager) IsStreamsEnabled() bool {
	return rd.isStreamsEnabled
}

func (rd *HandlingDataManager) Shutdown() {
	if rd.shutdown != nil {
		rd.shutdown()
	}
}

func (rd *HandlingDataManager) SetHandleRoutes(mux *http.ServeMux) {
	if rd.isStreamsEnabled {
		mux.HandleFunc(
			"/load_flows",
			rd.handleFlowsLoading(),
		)
	} else {
		mux.HandleFunc(
			"/apply_policies",
			HandleApplyPolicies(
				rd.configBuildResult.Accessor,
				rd.writer),
		)
		mux.HandleFunc(
			"/validate_policies",
			HandleValidatePolicies(),
		)
	}
	mux.HandleFunc(
		"/discover",
		HandleJSONFileRead(environment.GetDiscoveryStateLocation()),
	)

	mux.HandleFunc(
		"/remedy_stats",
		HandleJSONFileRead(environment.GetRemedyStateLocation()),
	)

	mux.HandleFunc(
		"/handshake",
		HandleHandshake(),
	)
}

func (rd *HandlingDataManager) initializeStreams() (err error) {
	log.Info().Msg("Using streams for Lunar Engine")

	rd.isStreamsEnabled = true

	var previousHaProxyReq *config.HAProxyEndpointsRequest
	if rd.stream != nil {
		previousHaProxyReq = rd.buildHAProxyFlowsEndpointsRequest()
	}

	// Shutdown if OpenTelemetry provider is already initialized
	// This happens when calling load_flows
	rd.Shutdown()

	rd.shutdown = otel.InitProvider(lunarEngine)

	if !rd.areMetricsInitialized {
		go otel.ServeMetrics()
		rd.areMetricsInitialized = true
	}

	rd.stream = streams.NewStream()
	rd.stream.WithHub(rd.lunarHub)
	if err = rd.stream.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize streams: %w", err)
	}

	haProxyReq := rd.buildHAProxyFlowsEndpointsRequest()
	if err = config.ManageHAProxyEndpoints(haProxyReq); err != nil {
		return fmt.Errorf("failed to manage HAProxy endpoints: %w", err)
	}
	newHAProxyEndpoints := rd.buildHAProxyFlowsEndpointsRequest()

	err = config.ManageHAProxyEndpoints(newHAProxyEndpoints)
	if err != nil {
		return fmt.Errorf("failed to initialize HAProxy endpoints: %v", err)
	}

	// Unmanaging HAProxy endpoints should occur after all possible transactions have reached Engine
	if previousHaProxyReq != nil &&
		len(previousHaProxyReq.ManagedEndpoints) > 0 {
		haproxyEndpointsToRemove, _ := lo.Difference(
			previousHaProxyReq.ManagedEndpoints,
			newHAProxyEndpoints.ManagedEndpoints,
		)
		config.ScheduleUnmanageHAProxyEndpoints(haproxyEndpointsToRemove)
	}

	return nil
}

func (rd *HandlingDataManager) handleFlowsLoading() func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			err := rd.initializeStreams()
			if err != nil {
				handleError(writer,
					fmt.Sprintf("Failed to load flows: %v", err),
					http.StatusUnprocessableEntity, err)
				return
			}
			SuccessResponse(writer, "✅ Successfully loaded flows")
		default:
			http.Error(
				writer,
				"Unsupported Method",
				http.StatusMethodNotAllowed,
			)
		}
	}
}

func (rd *HandlingDataManager) initializePolicies() error {
	log.Info().Msg("Using policies for Lunar Engine")
	sharedConfig.Validate.RegisterStructValidation(
		config.ValidateStructLevel,
		sharedConfig.Remedy{},         //nolint: exhaustruct
		sharedConfig.Diagnosis{},      //nolint: exhaustruct
		sharedConfig.PoliciesConfig{}, //nolint: exhaustruct
	)
	err := sharedConfig.Validate.RegisterValidation(
		"validateInt",
		config.ValidateInt,
	)
	if err != nil {
		return fmt.Errorf("failed to register config validation: %w", err)
	}

	configBuildResult, err := config.BuildInitialFromFile()
	if err != nil {
		return fmt.Errorf("failed to build initial config: %w", err)
	}
	rd.configBuildResult = configBuildResult
	rd.diagnosisWorker = runner.NewDiagnosisWorker()

	rd.shutdown = otel.InitProvider(lunarEngine)

	go otel.ServeMetrics()

	rd.policiesServices, err = services.Initialize(
		rd.writer,
		rd.proxyTimeout,
		rd.configBuildResult.Initial.Config.Exporters,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}
	return nil
}

func (rd *HandlingDataManager) buildHAProxyFlowsEndpointsRequest() *config.HAProxyEndpointsRequest {
	if !rd.isStreamsEnabled {
		return &config.HAProxyEndpointsRequest{}
	}
	manageAll := false
	for _, filter := range rd.stream.GetSupportedFilters() {
		if len(filter) == 0 {
			continue
		}
		filter := filter[0]
		if filter.IsAnyURLAccepted() {
			manageAll = true
			break
		}
	}

	managedEndpoints := []string{}
	for _, filter := range rd.stream.GetSupportedFilters() {
		if len(filter) == 0 {
			continue
		}
		filter := filter[0]
		for _, method := range filter.GetSupportedMethods() {
			managedEndpoints = append(managedEndpoints,
				config.HaproxyEndpointFormat(method, filter.GetURL()))
		}
	}
	return &config.HAProxyEndpointsRequest{
		ManageAll:        manageAll,
		ManagedEndpoints: managedEndpoints,
	}
}
