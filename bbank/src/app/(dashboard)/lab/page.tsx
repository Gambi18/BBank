import AreaPlaceholder from '@/components/AreaPlaceholder'

export default function LaboratoryArea() {
    return (
        <AreaPlaceholder
            area="Laboratory"
            workItem="WI-47"
            does={[
                'Enter TTI and grouping results per assay',
                'Release a unit only when every mandatory test is non-reactive',
                'Quarantine or discard a unit, and record why',
            ]}
        />
    )
}
