import AreaPlaceholder from '@/components/AreaPlaceholder'

export default function HospitalArea() {
    return (
        <AreaPlaceholder
            area="Hospital"
            workItem="WI-66"
            does={[
                'Raise a blood request against available stock',
                "See aggregate availability — never another hospital's detail",
                'Record the transfusion outcome for each unit issued',
            ]}
        />
    )
}
