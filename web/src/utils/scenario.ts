import type { Scenario, ScenarioStep } from '../api/client'

export function newScenarioStep(index: number): ScenarioStep {
  return {
    name: `步骤-${index + 1}`,
    method: 'GET',
    url: '',
    body_tpl: '',
    headers: {},
    extract: {},
  }
}

export function cloneScenarioDefinition(definition: Scenario): Scenario {
  return {
    formula: definition.formula ?? '',
    steps: (definition.steps ?? []).map((step) => ({
      name: step.name ?? '',
      method: step.method ?? 'GET',
      url: step.url ?? '',
      body_tpl: step.body_tpl ?? '',
      headers: { ...(step.headers ?? {}) },
      extract: { ...(step.extract ?? {}) },
    })),
  }
}
