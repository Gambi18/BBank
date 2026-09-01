import AreaPlaceholder from '@/components/AreaPlaceholder'

export default function StaffArea() {
    return (
        <AreaPlaceholder
            area="Staff"
            workItem="WI-39"
            does={[
                'Find a donor and check them in',
                'Record screening vitals, and defer when they fall out of range',
                'Record a collection and open the blood unit',
            ]}
        />
    )
}
