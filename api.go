package main

import (
	"errors"
	"fmt"
	"github.com/getgauge/common"
	"github.com/getgauge/gauge/gauge_messages"
	"github.com/golang/protobuf/proto"
	"log"
	"net"
	"path"
	"strconv"
	"strings"
	"sync"
)

type stepValue struct {
	args                   []string
	stepValue              string
	parameterizedStepValue string
}

func requestForSteps(runner *testRunner) []string {
	message, err := getResponseForGaugeMessage(createGetStepNamesRequest(), runner.connection)
	if err == nil {
		allStepsResponse := message.GetStepNamesResponse()
		return allStepsResponse.GetSteps()
	}
	return make([]string, 0)
}

func createGetStepNamesRequest() *gauge_messages.Message {
	return &gauge_messages.Message{MessageType: gauge_messages.Message_StepNamesRequest.Enum(), StepNamesRequest: &gauge_messages.StepNamesRequest{}}
}

func startAPIService(port int) error {
	specInfoGatherer := new(specInfoGatherer)
	apiHandler := &gaugeApiMessageHandler{specInfoGatherer}
	gaugeConnectionHandler, err := newGaugeConnectionHandler(port, apiHandler)
	if err != nil {
		return err
	}
	if port == 0 {
		if err := common.SetEnvVariable(common.ApiPortEnvVariableName, strconv.Itoa(gaugeConnectionHandler.connectionPortNumber())); err != nil {
			return errors.New(fmt.Sprintf("Failed to set Env variable %s. %s", common.ApiPortEnvVariableName, err.Error()))
		}
	}
	go gaugeConnectionHandler.handleMultipleConnections()
	specInfoGatherer.makeListOfAvailableSteps(nil)
	return nil
}

func runAPIServiceIndefinitely(port int, wg *sync.WaitGroup) {
	wg.Add(1)
	startAPIService(port)
}

type gaugeApiMessageHandler struct {
	specInfoGatherer *specInfoGatherer
}

func (handler *gaugeApiMessageHandler) messageBytesReceived(bytesRead []byte, conn net.Conn) {
	apiMessage := &gauge_messages.APIMessage{}
	var responseMessage *gauge_messages.APIMessage
	err := proto.Unmarshal(bytesRead, apiMessage)
	if err != nil {
		log.Printf("[Warning] Failed to read API proto message: %s\n", err.Error())
		responseMessage = handler.getErrorMessage(err)
	} else {
		messageType := apiMessage.GetMessageType()
		switch messageType {
		case gauge_messages.APIMessage_GetProjectRootRequest:
			responseMessage = handler.projectRootRequestResponse(apiMessage)
			break
		case gauge_messages.APIMessage_GetInstallationRootRequest:
			responseMessage = handler.installationRootRequestResponse(apiMessage)
			break
		case gauge_messages.APIMessage_GetAllStepsRequest:
			responseMessage = handler.getAllStepsRequestResponse(apiMessage)
			break
		case gauge_messages.APIMessage_GetAllSpecsRequest:
			responseMessage = handler.getAllSpecsRequestResponse(apiMessage)
			break
		case gauge_messages.APIMessage_GetStepValueRequest:
			responseMessage = handler.getStepValueRequestResponse(apiMessage)
			break
		case gauge_messages.APIMessage_GetLanguagePluginLibPathRequest:
			responseMessage = handler.getLanguagePluginLibPath(apiMessage)
			break
		case gauge_messages.APIMessage_GetAllConceptsRequest:
			responseMessage = handler.getAllConceptsRequestResponse(apiMessage)
			break
		}
	}
	handler.sendMessage(responseMessage, conn)
}

func (handler *gaugeApiMessageHandler) sendMessage(message *gauge_messages.APIMessage, conn net.Conn) {
	dataBytes, err := proto.Marshal(message)
	if err != nil {
		fmt.Printf("[Warning] Failed to respond to API request. Could not Marshal response %s\n", err.Error())
	}
	if err := write(conn, dataBytes); err != nil {
		fmt.Printf("[Warning] Failed to respond to API request. Could not write response %s\n", err.Error())
	}
}

func (handler *gaugeApiMessageHandler) projectRootRequestResponse(message *gauge_messages.APIMessage) *gauge_messages.APIMessage {
	root, err := common.GetProjectRoot()
	if err != nil {
		fmt.Printf("[Warning] Failed to find project root while responding to API request. %s\n", err.Error())
		root = ""
	}
	projectRootResponse := &gauge_messages.GetProjectRootResponse{ProjectRoot: proto.String(root)}
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_GetProjectRootResponse.Enum(), MessageId: message.MessageId, ProjectRootResponse: projectRootResponse}

}

func (handler *gaugeApiMessageHandler) installationRootRequestResponse(message *gauge_messages.APIMessage) *gauge_messages.APIMessage {
	root, err := common.GetInstallationPrefix()
	if err != nil {
		fmt.Printf("[Warning] Failed to find installation root while responding to API request. %s\n", err.Error())
		root = ""
	}
	installationRootResponse := &gauge_messages.GetInstallationRootResponse{InstallationRoot: proto.String(root)}
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_GetInstallationRootResponse.Enum(), MessageId: message.MessageId, InstallationRootResponse: installationRootResponse}
}

func (handler *gaugeApiMessageHandler) getAllStepsRequestResponse(message *gauge_messages.APIMessage) *gauge_messages.APIMessage {
	stepValues := handler.specInfoGatherer.getAvailableSteps()
	stepValueResponses := make([]*gauge_messages.ProtoStepValue, 0)
	for _, stepValue := range stepValues {
		stepValueResponses = append(stepValueResponses, convertToProtoStepValue(stepValue))
	}
	getAllStepsResponse := &gauge_messages.GetAllStepsResponse{AllSteps: stepValueResponses}
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_GetAllStepResponse.Enum(), MessageId: message.MessageId, AllStepsResponse: getAllStepsResponse}
}

func (handler *gaugeApiMessageHandler) getAllSpecsRequestResponse(message *gauge_messages.APIMessage) *gauge_messages.APIMessage {
	getAllSpecsResponse := handler.createGetAllSpecsResponseMessageFor(handler.specInfoGatherer.availableSpecs)
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_GetAllSpecsResponse.Enum(), MessageId: message.MessageId, AllSpecsResponse: getAllSpecsResponse}
}

func (handler *gaugeApiMessageHandler) getStepValueRequestResponse(message *gauge_messages.APIMessage) *gauge_messages.APIMessage {
	request := message.GetStepValueRequest()
	stepText := request.GetStepText()
	hasInlineTable := request.GetHasInlineTable()
	stepValue, err := extractStepValueAndParams(stepText, hasInlineTable)

	if err != nil {
		return handler.getErrorResponse(message, err)
	}
	stepValueResponse := &gauge_messages.GetStepValueResponse{StepValue: convertToProtoStepValue(stepValue)}
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_GetStepValueResponse.Enum(), MessageId: message.MessageId, StepValueResponse: stepValueResponse}

}

func (handler *gaugeApiMessageHandler) getAllConceptsRequestResponse(message *gauge_messages.APIMessage) *gauge_messages.APIMessage {
	allConceptsResponse := handler.createGetAllConceptsResponseMessageFor(handler.specInfoGatherer.conceptInfos)
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_GetAllConceptsResponse.Enum(), MessageId: message.MessageId, AllConceptsResponse: allConceptsResponse}

}

func (handler *gaugeApiMessageHandler) getLanguagePluginLibPath(message *gauge_messages.APIMessage) *gauge_messages.APIMessage {
	libPathRequest := message.GetLibPathRequest()
	language := libPathRequest.GetLanguage()
	languageInstallDir, err := common.GetPluginInstallDir(language, "")
	if err != nil {
		return handler.getErrorMessage(err)
	}
	runnerInfo, err := getRunnerInfo(language)
	if err != nil {
		return handler.getErrorMessage(err)
	}
	relativeLibPath := runnerInfo.Lib
	libPath := path.Join(languageInstallDir, relativeLibPath)
	response := &gauge_messages.GetLanguagePluginLibPathResponse{Path: proto.String(libPath)}
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_GetLanguagePluginLibPathResponse.Enum(), MessageId: message.MessageId, LibPathResponse: response}
}

func (handler *gaugeApiMessageHandler) getErrorResponse(message *gauge_messages.APIMessage, err error) *gauge_messages.APIMessage {
	errorResponse := &gauge_messages.ErrorResponse{Error: proto.String(err.Error())}
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_ErrorResponse.Enum(), MessageId: message.MessageId, Error: errorResponse}

}

func (handler *gaugeApiMessageHandler) getErrorMessage(err error) *gauge_messages.APIMessage {
	id := common.GetUniqueId()
	errorResponse := &gauge_messages.ErrorResponse{Error: proto.String(err.Error())}
	return &gauge_messages.APIMessage{MessageType: gauge_messages.APIMessage_ErrorResponse.Enum(), MessageId: &id, Error: errorResponse}
}

func (handler *gaugeApiMessageHandler) createGetAllSpecsResponseMessageFor(specs []*specification) *gauge_messages.GetAllSpecsResponse {
	protoSpecs := make([]*gauge_messages.ProtoSpec, 0)
	for _, spec := range specs {
		protoSpecs = append(protoSpecs, convertToProtoSpec(spec))
	}
	return &gauge_messages.GetAllSpecsResponse{Specs: protoSpecs}
}

func (handler *gaugeApiMessageHandler) createGetAllConceptsResponseMessageFor(conceptInfos []*gauge_messages.ConceptInfo) *gauge_messages.GetAllConceptsResponse {
	return &gauge_messages.GetAllConceptsResponse{Concepts: conceptInfos}
}

func extractStepValueAndParams(stepText string, hasInlineTable bool) (*stepValue, error) {
	stepValueWithPlaceHolders, args, err := processStepText(stepText)
	if err != nil {
		return nil, err
	}

	extractedStepValue, _ := extractStepValueAndParameterTypes(stepValueWithPlaceHolders)
	if hasInlineTable {
		extractedStepValue += " " + PARAMETER_PLACEHOLDER
		args = append(args, string(tableArg))
	}
	parameterizedStepValue := getParameterizeStepValue(extractedStepValue, args)

	return &stepValue{args, extractedStepValue, parameterizedStepValue}, nil

}

func getParameterizeStepValue(stepValue string, params []string) string {
	for _, param := range params {
		stepValue = strings.Replace(stepValue, PARAMETER_PLACEHOLDER, "<"+param+">", 1)
	}
	return stepValue
}