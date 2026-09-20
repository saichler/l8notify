package services

import (
	"errors"

	common "github.com/saichler/l8common/go/common"
	"github.com/saichler/l8types/go/ifs"
	ntf "github.com/saichler/l8types/go/types/l8notify"
)

const (
	IntegrationServiceName = "IntegCfg"
	// NotifyServiceArea is shared by the Notify service (NotifyRecordService.go).
	NotifyServiceArea = byte(78)
)

// ActivateIntegrationConfig activates the IntegrationConfig CRUD service.
// Unlike NotifyRecord, IntegrationConfig is fully editable — rows are created,
// updated, and deleted by an admin, not auto-generated as a delivery log.
func ActivateIntegrationConfig(creds, dbname string, vnic ifs.IVNic) {
	sla := common.NewOrmSLA(IntegrationServiceName, NotifyServiceArea, "IntegrationId",
		&IntegrationConfigCallback{}, &ntf.IntegrationConfig{}, &ntf.IntegrationConfigList{})
	// Name is the logical identity every lookup uses (GetIntegrationConfig("smtp")),
	// so it is a unique key alongside the generated IntegrationId primary key.
	sla.SetUniqueKeys("Name")
	common.ActivateService(sla, creds, dbname, vnic)
}

type IntegrationConfigCallback struct{}

func (this *IntegrationConfigCallback) Before(elem interface{}, action ifs.Action, isNotification bool, vnic ifs.IVNic) (interface{}, bool, error) {
	if action == ifs.GET {
		return nil, true, nil
	}
	cfg, ok := elem.(*ntf.IntegrationConfig)
	if !ok {
		return nil, true, errors.New("invalid integration config type")
	}
	switch action {
	case ifs.POST:
		common.GenerateID(&cfg.IntegrationId)
		return cfg, true, nil
	case ifs.PUT, ifs.PATCH:
		return cfg, true, nil // editable — no immutability constraint, unlike NotifyRecord
	}
	return nil, true, nil
}

func (this *IntegrationConfigCallback) After(elem interface{}, action ifs.Action, notify bool, vnic ifs.IVNic) (interface{}, bool, error) {
	return nil, true, nil
}
