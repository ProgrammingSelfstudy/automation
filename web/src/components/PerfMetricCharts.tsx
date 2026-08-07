import type { PerfMonitoringSample } from '../api/perfAgent'
import PerfRealtimeChart from './PerfRealtimeChart'

// PerfMetricCharts：CPU/内存/FPS/Jank 四张曲线图的固定分组，"性能采集"
// 实时页面和"历史数据"回放详情共用同一套——两边看的是同样的字段，没道理
// 各画一套。maxSamples 透传给 PerfRealtimeChart：实时页面用默认的滚动
// 窗口，历史回放传 0 看完整这次采集的全貌。
export default function PerfMetricCharts({ samples, maxSamples }: { samples: PerfMonitoringSample[]; maxSamples?: number }) {
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <PerfRealtimeChart
        title="CPU"
        samples={samples}
        unit="%"
        fixedRange={[0, 100]}
        maxSamples={maxSamples}
        series={[
          { metric: 'app_cpu', label: 'App CPU', color: '#2a78d6' },
          { metric: 'total_cpu', label: '总 CPU', color: '#008300' },
        ]}
      />
      <PerfRealtimeChart
        title="内存"
        samples={samples}
        unit="MB"
        maxSamples={maxSamples}
        series={[
          { metric: 'memory_pss', label: 'PSS', color: '#2a78d6' },
          { metric: 'java_heap', label: 'Java Heap', color: '#008300' },
          { metric: 'native_heap', label: 'Native Heap', color: '#e87ba4' },
        ]}
      />
      <PerfRealtimeChart
        title="FPS"
        samples={samples}
        unit="fps"
        fixedRange={[0, 60]}
        maxSamples={maxSamples}
        series={[{ metric: 'fps', label: 'FPS', color: '#2a78d6' }]}
      />
      <PerfRealtimeChart
        title="Jank"
        samples={samples}
        unit="count"
        maxSamples={maxSamples}
        series={[
          { metric: 'jank', label: 'Jank', color: '#2a78d6' },
          { metric: 'big_jank', label: 'Big Jank', color: '#eb6834' },
        ]}
      />
    </div>
  )
}
