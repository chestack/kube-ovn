package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (m *PortStatus) addCondition(ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	c := &PortCondition{
		Type:               ctype,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
	m.Conditions = append(m.Conditions, *c)
}

// setConditionValue updates or creates a new condition
func (m *PortStatus) setConditionValue(ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	var c *PortCondition
	for i := range m.Conditions {
		if m.Conditions[i].Type == ctype {
			c = &m.Conditions[i]
		}
	}
	if c == nil {
		m.addCondition(ctype, status, reason, message)
	} else {
		// check message ?
		if c.Status == status && c.Reason == reason && c.Message == message {
			return
		}
		now := metav1.Now()
		c.LastUpdateTime = now
		if c.Status != status {
			c.LastTransitionTime = now
		}
		c.Status = status
		c.Reason = reason
		c.Message = message
	}
}

// RemoveCondition removes the condition with the provided type.
func (m *PortStatus) RemoveCondition(ctype ConditionType) {
	for i, c := range m.Conditions {
		if c.Type == ctype {
			m.Conditions[i] = m.Conditions[len(m.Conditions)-1]
			m.Conditions = m.Conditions[:len(m.Conditions)-1]
			break
		}
	}
}

// GetCondition get existing condition
func (m *PortStatus) GetCondition(ctype ConditionType) *PortCondition {
	for i := range m.Conditions {
		if m.Conditions[i].Type == ctype {
			return &m.Conditions[i]
		}
	}
	return nil
}

// IsConditionTrue - if condition is true
func (m *PortStatus) IsConditionTrue(ctype ConditionType) bool {
	if c := m.GetCondition(ctype); c != nil {
		return c.Status == corev1.ConditionTrue
	}
	return false
}

// IsReady returns true if ready condition is set
func (m *PortStatus) IsReady() bool { return m.IsConditionTrue(Ready) }

// IsNotReady returns true if ready condition is set
func (m *PortStatus) IsNotReady() bool { return !m.IsConditionTrue(Ready) }

// IsValidated returns true if ready condition is set
func (m *PortStatus) IsValidated() bool { return m.IsConditionTrue(Validated) }

// IsNotValidated returns true if ready condition is set
func (m *PortStatus) IsNotValidated() bool { return !m.IsConditionTrue(Validated) }

// ConditionReason - return condition reason
func (m *PortStatus) ConditionReason(ctype ConditionType) string {
	if c := m.GetCondition(ctype); c != nil {
		return c.Reason
	}
	return ""
}

// Ready - shortcut to set ready condition to true
func (m *PortStatus) Ready(reason, message string) {
	m.SetCondition(Ready, reason, message)
}

// NotReady - shortcut to set ready condition to false
func (m *PortStatus) NotReady(reason, message string) {
	m.ClearCondition(Ready, reason, message)
}

// Validated - shortcut to set validated condition to true
func (m *PortStatus) Validated(reason, message string) {
	m.SetCondition(Validated, reason, message)
}

// NotValidated - shortcut to set validated condition to false
func (m *PortStatus) NotValidated(reason, message string) {
	m.ClearCondition(Validated, reason, message)
}

// SetError - shortcut to set error condition
func (m *PortStatus) SetError(reason, message string) {
	m.SetCondition(Error, reason, message)
}

// ClearError - shortcut to set error condition
func (m *PortStatus) ClearError() {
	m.ClearCondition(Error, "NoError", "No error seen")
}

// EnsureCondition useful for adding default conditions
func (m *PortStatus) EnsureCondition(ctype ConditionType) {
	if c := m.GetCondition(ctype); c != nil {
		return
	}
	m.addCondition(ctype, corev1.ConditionUnknown, ReasonInit, "Not Observed")
}

// EnsureStandardConditions - helper to inject standard conditions
func (m *PortStatus) EnsureStandardConditions() {
	m.EnsureCondition(Ready)
	m.EnsureCondition(Validated)
	m.EnsureCondition(Error)
}

// ClearCondition updates or creates a new condition
func (m *PortStatus) ClearCondition(ctype ConditionType, reason, message string) {
	m.setConditionValue(ctype, corev1.ConditionFalse, reason, message)
}

// SetCondition updates or creates a new condition
func (m *PortStatus) SetCondition(ctype ConditionType, reason, message string) {
	m.setConditionValue(ctype, corev1.ConditionTrue, reason, message)
}

// RemoveAllConditions updates or creates a new condition
func (m *PortStatus) RemoveAllConditions() {
	m.Conditions = []PortCondition{}
}

// ClearAllConditions updates or creates a new condition
func (m *PortStatus) ClearAllConditions() {
	for i := range m.Conditions {
		m.Conditions[i].Status = corev1.ConditionFalse
	}
}

func (m *FipStatus) addCondition(ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	c := &FipCondition{
		Type:               ctype,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Status:             status,
		Reason:             reason,
		Message:            message,
	}
	m.Conditions = append(m.Conditions, *c)
}

// setConditionValue updates or creates a new condition
func (m *FipStatus) setConditionValue(ctype ConditionType, status corev1.ConditionStatus, reason, message string) {
	var c *FipCondition
	for i := range m.Conditions {
		if m.Conditions[i].Type == ctype {
			c = &m.Conditions[i]
		}
	}
	if c == nil {
		m.addCondition(ctype, status, reason, message)
	} else {
		// check message ?
		if c.Status == status && c.Reason == reason && c.Message == message {
			return
		}
		now := metav1.Now()
		c.LastUpdateTime = now
		if c.Status != status {
			c.LastTransitionTime = now
		}
		c.Status = status
		c.Reason = reason
		c.Message = message
	}
}

// RemoveCondition removes the condition with the provided type.
func (m *FipStatus) RemoveCondition(ctype ConditionType) {
	for i, c := range m.Conditions {
		if c.Type == ctype {
			m.Conditions[i] = m.Conditions[len(m.Conditions)-1]
			m.Conditions = m.Conditions[:len(m.Conditions)-1]
			break
		}
	}
}

// GetCondition get existing condition
func (m *FipStatus) GetCondition(ctype ConditionType) *FipCondition {
	for i := range m.Conditions {
		if m.Conditions[i].Type == ctype {
			return &m.Conditions[i]
		}
	}
	return nil
}

// IsConditionTrue - if condition is true
func (m *FipStatus) IsConditionTrue(ctype ConditionType) bool {
	if c := m.GetCondition(ctype); c != nil {
		return c.Status == corev1.ConditionTrue
	}
	return false
}

// IsReady returns true if ready condition is set
func (m *FipStatus) IsReady() bool { return m.IsConditionTrue(Ready) }

// IsNotReady returns true if ready condition is set
func (m *FipStatus) IsNotReady() bool { return !m.IsConditionTrue(Ready) }

// IsValidated returns true if ready condition is set
func (m *FipStatus) IsValidated() bool { return m.IsConditionTrue(Validated) }

// IsNotValidated returns true if ready condition is set
func (m *FipStatus) IsNotValidated() bool { return !m.IsConditionTrue(Validated) }

// ConditionReason - return condition reason
func (m *FipStatus) ConditionReason(ctype ConditionType) string {
	if c := m.GetCondition(ctype); c != nil {
		return c.Reason
	}
	return ""
}

// Ready - shortcut to set ready condition to true
func (m *FipStatus) Ready(reason, message string) {
	m.SetCondition(Ready, reason, message)
}

// NotReady - shortcut to set ready condition to false
func (m *FipStatus) NotReady(reason, message string) {
	m.ClearCondition(Ready, reason, message)
}

// Validated - shortcut to set validated condition to true
func (m *FipStatus) Validated(reason, message string) {
	m.SetCondition(Validated, reason, message)
}

// NotValidated - shortcut to set validated condition to false
func (m *FipStatus) NotValidated(reason, message string) {
	m.ClearCondition(Validated, reason, message)
}

// SetError - shortcut to set error condition
func (m *FipStatus) SetError(reason, message string) {
	m.SetCondition(Error, reason, message)
}

// ClearError - shortcut to set error condition
func (m *FipStatus) ClearError() {
	m.ClearCondition(Error, "NoError", "No error seen")
}

// EnsureCondition useful for adding default conditions
func (m *FipStatus) EnsureCondition(ctype ConditionType) {
	if c := m.GetCondition(ctype); c != nil {
		return
	}
	m.addCondition(ctype, corev1.ConditionUnknown, ReasonInit, "Not Observed")
}

// EnsureStandardConditions - helper to inject standard conditions
func (m *FipStatus) EnsureStandardConditions() {
	m.EnsureCondition(Ready)
	m.EnsureCondition(Validated)
	m.EnsureCondition(Error)
}

// ClearCondition updates or creates a new condition
func (m *FipStatus) ClearCondition(ctype ConditionType, reason, message string) {
	m.setConditionValue(ctype, corev1.ConditionFalse, reason, message)
}

// SetCondition updates or creates a new condition
func (m *FipStatus) SetCondition(ctype ConditionType, reason, message string) {
	m.setConditionValue(ctype, corev1.ConditionTrue, reason, message)
}

// RemoveAllConditions updates or creates a new condition
func (m *FipStatus) RemoveAllConditions() {
	m.Conditions = []FipCondition{}
}

// ClearAllConditions updates or creates a new condition
func (m *FipStatus) ClearAllConditions() {
	for i := range m.Conditions {
		m.Conditions[i].Status = corev1.ConditionFalse
	}
}
