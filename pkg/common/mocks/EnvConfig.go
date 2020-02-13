// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// EnvConfig is an autogenerated mock type for the EnvConfig type
type EnvConfig struct {
	mock.Mock
}

// GetEnvFloatVar provides a mock function with given fields: varName, defaultValue
func (_m *EnvConfig) GetEnvFloatVar(varName string, defaultValue float64) float64 {
	ret := _m.Called(varName, defaultValue)

	var r0 float64
	if rf, ok := ret.Get(0).(func(string, float64) float64); ok {
		r0 = rf(varName, defaultValue)
	} else {
		r0 = ret.Get(0).(float64)
	}

	return r0
}

// GetEnvIntVar provides a mock function with given fields: varName, defaultValue
func (_m *EnvConfig) GetEnvIntVar(varName string, defaultValue int) int {
	ret := _m.Called(varName, defaultValue)

	var r0 int
	if rf, ok := ret.Get(0).(func(string, int) int); ok {
		r0 = rf(varName, defaultValue)
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// GetEnvVar provides a mock function with given fields: varName, defaultValue
func (_m *EnvConfig) GetEnvVar(varName string, defaultValue string) string {
	ret := _m.Called(varName, defaultValue)

	var r0 string
	if rf, ok := ret.Get(0).(func(string, string) string); ok {
		r0 = rf(varName, defaultValue)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// RequireEnvIntVar provides a mock function with given fields: varName
func (_m *EnvConfig) RequireEnvIntVar(varName string) int {
	ret := _m.Called(varName)

	var r0 int
	if rf, ok := ret.Get(0).(func(string) int); ok {
		r0 = rf(varName)
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// RequireEnvVar provides a mock function with given fields: varName
func (_m *EnvConfig) RequireEnvVar(varName string) string {
	ret := _m.Called(varName)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(varName)
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}